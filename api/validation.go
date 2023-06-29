package api

import (
	"context"
	"errors"
	"github.com/gorilla/websocket"
	"proofofaccess/hive"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"proofofaccess/proofcrypto"
	"proofofaccess/pubsub"
	"proofofaccess/validation"
	"strconv"
	"time"
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
	sendWsResponse("Connecting", "Connecting to Peer", "0", conn)
	var err error
	log.Info(peerID)
	err = ipfs.IpfsPingNode(peerID)
	if err != nil {
		sendWsResponse("PeerNotFound", "Peer Not Found", "0", conn)
		log.Error(err)
		return err
	}
	sendWsResponse("Connected", "Connected to Peer", "0", conn)
	salt := msg.SALT
	if salt == "" {
		salt, err = proofcrypto.CreateRandomHash()
		if err != nil {
			sendWsResponse(wsError, "Failed to create random hash", "0", conn)
			log.Error(err)
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout*maxPingAttempts)
	defer cancel()
	return performPingPong(ctx, salt, msg.Name, conn)
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
			messaging.PingPong(salt, name)
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

func createProofRequest(salt string, CID string, conn *websocket.Conn) ([]byte, error) {
	localdata.SetStatus(salt, CID, "Pending")
	proofJson, err := validation.ProofRequestJson(salt, CID)
	if err != nil {
		sendWsResponse(wsError, "Failed to create proof request JSON", "0", conn)
		log.Error(err)
		return nil, err
	}

	return proofJson, nil
}

func sendProofRequest(salt string, proofJson []byte, name string, conn *websocket.Conn) error {
	sendWsResponse(wsRequestingProof, "RequestingProof", "0", conn)
	if localdata.WsPeers[name] == name && localdata.NodeType == 1 {
		ws := localdata.WsClients[name]
		ws.WriteMessage(websocket.TextMessage, proofJson)
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		localdata.WsValidators["Validator1"].WriteMessage(websocket.TextMessage, proofJson)
	} else {
		err := pubsub.Publish(string(proofJson), name)
		if err != nil {
			sendWsResponse(wsError, "Failed to send proof request to storage node", "0", conn)
			log.Error(err)
			return err
		}
	}
	localdata.SaveTime(salt)
	return nil
}

func waitForProofStatus(salt string, cid string, conn *websocket.Conn) (string, time.Duration, error) {
	sendWsResponse("Waiting Proof", "Waiting for Proof", "0", conn)
	proofTimeout := time.NewTicker(validationTimeout)
	defer proofTimeout.Stop()

	proofStart := make(chan struct{}, 1)
	defer close(proofStart)

	go func() {
		for {
			if messaging.ProofRequest[salt] {
				sendWsResponse(wsProofReceived, "ProofReceived", "0", conn)
				sendWsResponse(wsValidating, "Validating", "0", conn)
				return
			}
			//wait 1 second before checking again
			time.Sleep(100 * time.Millisecond)
		}
	}()

	proofDone := make(chan struct{})
	defer close(proofDone)

	go func() {
		for {
			select {
			case <-proofTimeout.C:
				if messaging.ProofRequestStatus[salt] {
					sendWsResponse(localdata.GetStatus(salt).Status, localdata.GetStatus(salt).Status, localdata.GetElapsed(salt).String(), conn)
					close(proofDone)
					return
				}
			case <-proofDone:
				return
			}
		}
	}()
	<-proofDone
	ctx, cancel := context.WithTimeout(context.Background(), validationTimeout)
	defer cancel()
	status, elapsed, err := getStatusAndElapsed(ctx, salt)
	if err != nil {
		return "", 0, err
	}
	return status, elapsed, nil
}

func getStatusAndElapsed(ctx context.Context, salt string) (string, time.Duration, error) {
	for {
		select {
		case <-ctx.Done():
			return "", 0, ctx.Err()
		default:
			status := localdata.GetStatus(salt).Status
			if status != "Pending" {
				elapsed := localdata.GetElapsed(salt)
				return status, elapsed, nil
			}
			time.Sleep(validationTimeout)
		}
	}
}

func formatElapsed(elapsed time.Duration) string {
	return strconv.FormatFloat(float64(elapsed.Milliseconds()), 'f', 0, 64) + "ms"
}
