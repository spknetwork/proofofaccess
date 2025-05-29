package api

import (
	"context"
	"errors"
	"fmt"
	"proofofaccess/hive"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"proofofaccess/proofcrypto"
	"proofofaccess/pubsub"
	"proofofaccess/validation"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func getPeerID(msg *message, conn *websocket.Conn) (string, error) {
	name := msg.Name
	peerID := msg.PEERID
	var err error
	if peerID == "" && name != "" {
		sendWsResponse("FetchingHiveAccount", "Fetching Peer ID from Hive", "0", conn)
		peerID, err = hive.GetIpfsID(name)
		if err != nil {
			sendWsResponse("IpfsPeerIDError", "Please enable Proof of Access and register your ipfs node to your hive account", "0", conn)
			log.Error(err)
			return "", err
		}
	}
	sendWsResponse("FoundHiveAccount", "Found Hive Account", "0", conn)
	return peerID, nil
}

func connectToPeer(peerID string, conn *websocket.Conn, msg *message) error {
	sendWsResponse("Connecting", "Attempting direct IPFS connection", "0", conn)
	var err error
	log.Debug(peerID)
	err = ipfs.IpfsPingNode(peerID)
	if err != nil {
		sendWsResponse("DirectConnectionFailed", "Direct IPFS connection failed, using network routing", "0", conn)
		log.Debugf("Direct IPFS connection to peer %s failed: %v", peerID, err)
		log.Debugf("Will attempt proof validation via PubSub/WebSocket instead")
		// Continue with PubSub ping instead of returning error
	} else {
		sendWsResponse("Connected", "Direct IPFS connection established", "0", conn)
		log.Debugf("Direct IPFS connection to peer %s successful", peerID)
	}

	// Always attempt PubSub ping regardless of direct IPFS connection result
	salt := msg.SALT
	if salt == "" {
		salt, err = proofcrypto.CreateRandomHash()
		if err != nil {
			sendWsResponse(wsError, "Failed to create random hash", "0", conn)
			log.Error(err)
			return err
		}
	}

	sendWsResponse("NetworkPing", "Testing network connectivity via PubSub", "0", conn)
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout*maxPingAttempts)
	defer cancel()
	pingErr := performPingPong(ctx, salt, msg.Name, conn)
	if pingErr != nil {
		log.Debugf("PubSub ping to %s also failed: %v", msg.Name, pingErr)
		// If both direct IPFS and PubSub ping fail, that's more concerning
		if err != nil {
			return fmt.Errorf("both direct IPFS connection and PubSub ping failed for %s", msg.Name)
		}
		// If only PubSub ping failed but direct IPFS worked, still proceed
		return pingErr
	}

	sendWsResponse("NetworkConnected", "Network connectivity confirmed", "0", conn)
	return nil
}

func sendWsResponseAndHandleError(status string, message string, elapsed string, conn *websocket.Conn, err error) error {
	sendWsResponse(status, message, elapsed, conn)
	if err != nil {
		log.Error(err)
		return err
	}
	return nil
}

func performPingPong(ctx context.Context, salt string, name string, conn *websocket.Conn) error {
	attempts := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if messaging.Ping[salt] {
				return nil
			}
			messaging.SendPing(salt, name, conn)
			attempts++
			if attempts > maxPingAttempts {
				return sendWsResponseAndHandleError("Connection Error", "Could not connect to peer, try again", "0", conn, errors.New("connection error"))
			}
			time.Sleep(pingTimeout)
		}
	}
}

func createRandomHash(conn *websocket.Conn) (string, error) {
	hash, err := proofcrypto.CreateRandomHash()
	if err != nil {
		sendWsResponse(wsError, "Failed to create random seed hash", "0", conn)
		log.Error(err)
		return "", err
	}

	return hash, nil
}

func createProofRequest(salt string, CID string, conn *websocket.Conn, name string) ([]byte, error) {
	//fmt.Println(name)
	localdata.Lock.Lock()
	// Call SetStatus with new signature for initial Pending status
	// Pass zero time and zero duration initially.
	localdata.SetStatus(salt, CID, "Pending", name, time.Time{}, 0)
	localdata.Lock.Unlock()
	//fmt.Println("createProofRequest2")
	proofJson, err := validation.ProofRequestJson(salt, CID)
	if err != nil {
		sendWsResponse(wsError, "Failed to create proof request JSON", "0", conn)
		log.Error(err)
		return nil, err
	}

	return proofJson, nil
}

func sendProofRequest(salt string, cid string, proofJson []byte, target string, conn *websocket.Conn) error {
	sendWsResponse(wsRequestingProof, "RequestingProof", "0", conn)

	// Determine if target is a peer ID (starts with "12D3KooW") or username
	isPeerID := strings.HasPrefix(target, "12D3KooW")
	var username string
	var peerID string

	if isPeerID {
		peerID = target
		// For now, we need to extract username from the original message context
		// This is a limitation - we'll use PeerID for tracking but username for communication
		username = target // Fallback - this will need to be improved
	} else {
		username = target
		peerID = target // Fallback
	}

	log.Infof("Attempting to send proof request to target: %s (isPeerID: %v)", target, isPeerID)
	log.Debugf("  Target details - Username: %s, PeerID: %s", username, peerID)
	log.Debugf("  CID: %s, Salt: %s", cid, salt)
	log.Debugf("  Current node type: %d, UseWS: %v", localdata.NodeType, localdata.UseWS)

	// Check WebSocket peer connection for validators sending to storage nodes
	if localdata.WsPeers[username] == username && localdata.NodeType == 1 {
		log.Infof("Sending proof request to storage node %s via WebSocket (validator->storage)", username)
		ws := localdata.WsClients[username]
		if ws == nil {
			log.Errorf("WebSocket connection to %s is nil", username)
			return errors.New("WebSocket connection is nil")
		}
		err := ws.WriteMessage(websocket.TextMessage, proofJson)
		if err != nil {
			log.Errorf("Failed to send proof request via WebSocket to %s: %v", username, err)
			return err
		}
		log.Infof("Proof request sent successfully via WebSocket to %s (peer: %s)", username, peerID)
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		log.Infof("Sending proof request to validator %s via WebSocket (storage->validator)", username)
		localdata.Lock.Lock()
		validatorWs := localdata.WsValidators[username]
		localdata.Lock.Unlock()
		if validatorWs == nil {
			log.Errorf("Validator WebSocket connection to %s is nil", username)
			return errors.New("Validator WebSocket connection is nil")
		}
		err := validatorWs.WriteMessage(websocket.TextMessage, proofJson)
		if err != nil {
			log.Errorf("Failed to send proof request via WebSocket to validator %s: %v", username, err)
			return err
		}
		log.Infof("Proof request sent successfully via WebSocket to validator %s (peer: %s)", username, peerID)
	} else {
		// For PubSub, we must use username since that's what nodes subscribe to
		if isPeerID {
			log.Warnf("Cannot send to peer ID %s via PubSub - PubSub uses usernames for topics", peerID)
			log.Warnf("This indicates multiple IPFS instances for the same user - functionality not fully supported yet")
			return fmt.Errorf("cannot target specific peer ID %s via PubSub - need username", peerID)
		}

		log.Infof("Sending proof request to %s via PubSub", username)
		err := pubsub.Publish(string(proofJson), username)
		if err != nil {
			log.Errorf("Failed to send proof request via PubSub to %s: %v", username, err)
			sendWsResponse(wsError, "Failed to send proof request to storage node", "0", conn)
			return err
		}
		log.Infof("Proof request sent successfully via PubSub to %s", username)
	}

	// Call SaveTime with CID and salt
	localdata.SaveTime(cid, salt)
	log.Debugf("Saved request time for CID %s, salt %s", cid, salt)
	return nil
}
