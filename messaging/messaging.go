package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	poaipfs "proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/pubsub"
	"sync"
	"time"
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

var (
	log = logrus.New()
)
var wsMutex = &sync.Mutex{}
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
func HandleMessage(message string) {
	// JSON decode message
	req := Request{}
	err := json.Unmarshal([]byte(message), &req)
	if err != nil {
		fmt.Println("Error decoding JSON:", err)
		return
	}
	//fmt.Println("Message received:", message)
	//Handle proof of access response from storage node
	localdata.Lock.Lock()
	nodeType := localdata.NodeType
	localdata.Lock.Unlock()
	if nodeType == 1 {
		if req.Type == TypeProofOfAccess {
			fmt.Println("Proof of access received")
			fmt.Println("Hash: " + req.Hash)
			fmt.Println("Seed: " + req.Seed)
			fmt.Println("User: " + req.User)
			fmt.Println("CID: " + req.CID)
			fmt.Println("Pins: " + req.Pins)
			go HandleProofOfAccess(req)
		}

	}

	//Handle request for proof request from validation node
	if nodeType == 2 {
		if req.Type == TypeRequestProof {
			go HandleRequestProof(req)
		}
		if req.Type == TypePingPongPong {
			validatorName := req.User
			localdata.Lock.Lock()
			localdata.Validators[validatorName] = true
			localdata.Lock.Unlock()
			fmt.Println("Validator", validatorName, "is online")
		}
	}
	if req.Type == TypePingPongPong {
		Ping[req.Hash] = true
		fmt.Println("PingPongPong received")
	}
	if req.Type == TypePingPongPing {
		fmt.Println("PingPongPing received")
		PingPongPong(req, req.Hash, req.User)
		nodeStatus := localdata.NodesStatus[req.User]
		nodes := Nodes[req.User]
		if nodeType == 1 && !nodes && nodeStatus != "Synced" {
			fmt.Println("syncing: " + req.User)
			go RequestCIDS(req)
		}

	}
	if req.Type == "RequestCIDS" {
		fmt.Println("RequestCIDS received")
		go SendCIDS(req.User)

	}
	if req.Type == "SendCIDS" {
		fmt.Println("SendCIDS received")
		go SyncNode(req)

	}
	if req.Type == "Syncing" {
		fmt.Println("Syncing received")
		go ReceiveSyncing(req)
	}
	if req.Type == "Synced" {
		fmt.Println("Synced with " + req.User)
		localdata.Synced = true
		go ReceiveSynced(req)
	}
	// fmt.Println("Message handled")

}
func PubsubHandler(ctx context.Context) {
	if poaipfs.Shell != nil {
		sub, err := pubsub.Subscribe(localdata.NodeName)
		if err != nil {
			log.Error("Error subscribing to pubsub: ", err)
			return
		}

		log.Info("User:", localdata.NodeName)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := pubsub.Read(sub)
				if err != nil {
					log.Error("Error reading from pubsub: ", err)
					continue
				}
				HandleMessage(msg)
			}
		}
	} else {
		time.Sleep(1 * time.Second)
	}
}
func SendPing(hash string, user string) {
	data := map[string]string{
		"type": TypePingPongPing,
		"hash": hash,
		"user": localdata.GetNodeName(),
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error encoding JSON:", err)
		return
	}
	localdata.PingTime[user] = time.Now()
	if localdata.WsPeers[user] == user && localdata.NodeType == 1 {
		wsMutex.Lock()
		ws := localdata.WsClients[user]
		ws.WriteMessage(websocket.TextMessage, jsonData)
		wsMutex.Unlock()
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		wsMutex.Lock()
		localdata.WsValidators[user].WriteMessage(websocket.TextMessage, jsonData)
		wsMutex.Unlock()
	} else {
		pubsub.Publish(string(jsonData), user)
	}
}
func PingPongPong(req Request, hash string, user string) {
	data := map[string]string{
		"type": TypePingPongPong,
		"hash": hash,
		"user": localdata.GetNodeName(),
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error encoding JSON:", err)
		return
	}
	if localdata.WsPeers[req.User] == req.User && localdata.NodeType == 1 {
		wsMutex.Lock()
		ws := localdata.WsClients[req.User]
		localdata.PeerLastActive[req.User] = time.Now()
		ws.WriteMessage(websocket.TextMessage, jsonData)
		wsMutex.Unlock()
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		wsMutex.Lock()
		localdata.WsValidators[req.User].WriteMessage(websocket.TextMessage, jsonData)
		wsMutex.Unlock()
	} else {
		pubsub.Publish(string(jsonData), user)
	}
}
