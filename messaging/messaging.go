package messaging

import (
	"context"
	"encoding/json"
	poaipfs "proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/pubsub"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

type Request struct {
	Type       string `json:"type"`
	Hash       string `json:"hash"`
	CID        string `json:"cid"`
	Seed       string `json:"seed"`
	User       string `json:"user"`
	Pins       string `json:"pins"`
	TotalParts string `json:"totalParts"`
	Part       string `json:"part"`
}

const (
	Layout            = "2006-01-02 15:04:05.999999 -0700 MST m=+0.000000000"
	TypeProofOfAccess = "ProofOfAccess"
	TypeRequestProof  = "RequestProof"
	TypePingPongPing  = "PingPongPing"
	TypePingPongPong  = "PingPongPong"
)

var ()
var WsMutex = &sync.Mutex{}
var Ping = map[string]bool{}
var ProofRequest = map[string]bool{}
var ProofRequestStatus = map[string]bool{}
var Nodes = map[string]bool{}
var PinFileCids = []string{}

type PinType struct {
	Type string `json:"Type"`
}

type PinMap map[string]PinType

// HandleMessage
// This is the function that handles the messages from the pubsub
func HandleMessage(message string, ws *websocket.Conn) {
	// JSON decode message
	req := Request{}
	err := json.Unmarshal([]byte(message), &req)
	if err != nil {
		logrus.Errorf("Error decoding JSON message: %v, Message: %s", err, message)
		return
	}
	//Handle proof of access response from storage node
	nodeType := localdata.NodeType
	if nodeType == 1 {
		if req.Type == TypeProofOfAccess {
			go HandleProofOfAccess(req, ws)
		}

	}

	//Handle request for proof request from validation node
	if nodeType == 2 {
		if req.Type == TypeRequestProof {
			go HandleRequestProof(req, ws)
		}
		if req.Type == TypePingPongPong {
			validatorName := req.User
			localdata.Lock.Lock()
			localdata.Validators[validatorName] = true
			localdata.Lock.Unlock()
		}
	}
	if req.Type == TypePingPongPong {
		Ping[req.Hash] = true
	}
	if req.Type == TypePingPongPing {
		PingPongPong(req, ws)
		nodeStatus := localdata.NodesStatus[req.User]
		nodes := Nodes[req.User]
		if nodeType == 1 && !nodes && nodeStatus != "Synced" {
			go SyncNode(req, ws)
		}

	}
	if req.Type == "RequestCIDS" {
		go SendCIDS(req.User, ws)

	}
	if req.Type == "SendCIDS" {
		go SyncNode(req, ws)

	}
	if req.Type == "Syncing" {
		go ReceiveSyncing(req)
	}
	if req.Type == "Synced" {
		localdata.Synced = true
		go ReceiveSynced(req)
	}

}
func PubsubHandler(ctx context.Context) {
	if poaipfs.Shell != nil {
		sub, err := pubsub.Subscribe(localdata.NodeName)
		if err != nil {
			logrus.Errorf("Error subscribing to pubsub topic %s: %v", localdata.NodeName, err)
			return
		}

		logrus.Infof("Subscribed to PubSub topic: %s", localdata.NodeName)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := pubsub.Read(sub)
				if err != nil {
					logrus.Errorf("Error reading from pubsub: %v", err)
					continue
				}
				var ws *websocket.Conn
				HandleMessage(msg, ws)
			}
		}
	} else {
		time.Sleep(1 * time.Second)
	}
}
func SendPing(hash string, user string, ws *websocket.Conn) {
	data := map[string]string{
		"type": TypePingPongPing,
		"hash": hash,
		"user": localdata.GetNodeName(),
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		logrus.Errorf("Error encoding Ping JSON: %v", err)
		return
	}
	localdata.PingTime[user] = time.Now()

	// Fix: Validator should use PubSub to send pings to storage nodes
	if localdata.NodeType == 1 {
		// Validator sending ping to storage node - always use PubSub
		logrus.Debugf("Validator sending ping to storage node %s via PubSub", user)
		pubsub.Publish(string(jsonData), user)
	} else if localdata.UseWS && localdata.NodeType == 2 {
		// Storage node sending ping to validator - use WebSocket
		WsMutex.Lock()
		err = localdata.WsValidators[user].WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			logrus.Errorf("Error writing Ping message to WebSocket validator %s: %v", user, err)
		}
		WsMutex.Unlock()
	} else {
		// Fallback to PubSub
		pubsub.Publish(string(jsonData), user)
	}
}
func PingPongPong(req Request, ws *websocket.Conn) {
	data := map[string]string{
		"type": TypePingPongPong,
		"hash": req.Hash,
		"user": localdata.GetNodeName(),
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		logrus.Errorf("Error encoding PingPongPong JSON: %v", err)
		return
	}
	if localdata.WsPeers[req.User] == req.User && localdata.NodeType == 1 {
		if ws == nil {
			logrus.Debugf("PingPongPong received for peer %s via PubSub (no WebSocket), publishing response", req.User)
			pubsub.Publish(string(jsonData), req.User)
			return
		}
		localdata.Lock.Lock()
		localdata.PeerLastActive[req.User] = time.Now()
		localdata.Lock.Unlock()
		WsMutex.Lock()
		err = ws.WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			logrus.Errorf("Error writing PingPongPong message to WebSocket for %s: %v", req.User, err)
		}
		WsMutex.Unlock()
	} else if localdata.UseWS && localdata.NodeType == 2 {
		WsMutex.Lock()
		if conn, ok := localdata.WsValidators[req.User]; ok && conn != nil {
			err := conn.WriteMessage(websocket.TextMessage, jsonData)
			if err != nil {
				logrus.Debugf("Error writing PingPongPong message to WebSocket validator %s: %v", req.User, err)
				localdata.Lock.Lock()
				delete(localdata.WsValidators, req.User)
				localdata.Lock.Unlock()
			}
		} else {
			logrus.Debugf("No valid WebSocket connection to validator %s in PingPongPong", req.User)
		}
		WsMutex.Unlock()
	} else {
		pubsub.Publish(string(jsonData), req.User)
	}
}
