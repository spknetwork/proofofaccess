package api

import (
	"context"
	"errors"
	"proofofaccess/hive"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"proofofaccess/proofcrypto"
	"proofofaccess/pubsub"
	"proofofaccess/validation"
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
	sendWsResponse("Connecting", "Connecting to Peer", "0", conn)
	var err error
	log.Debug(peerID)
	err = ipfs.IpfsPingNode(peerID)
	if err != nil {
		sendWsResponse("PeerNotFound", "Peer Not Found", "0", conn)
		log.Warnf("Peer %s not found via routing: %v", peerID, err)
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

func sendProofRequest(salt string, cid string, proofJson []byte, name string, conn *websocket.Conn) error {
	sendWsResponse(wsRequestingProof, "RequestingProof", "0", conn)
	if localdata.WsPeers[name] == name && localdata.NodeType == 1 {
		ws := localdata.WsClients[name]
		ws.WriteMessage(websocket.TextMessage, proofJson)
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		localdata.Lock.Lock()
		validatorWs := localdata.WsValidators[name]
		validatorWs.WriteMessage(websocket.TextMessage, proofJson)
		localdata.Lock.Unlock()
	} else {
		err := pubsub.Publish(string(proofJson), name)
		if err != nil {
			sendWsResponse(wsError, "Failed to send proof request to storage node", "0", conn)
			log.Error(err)
			return err
		}
	}
	// Call SaveTime with CID and salt
	localdata.SaveTime(cid, salt)
	return nil
}
