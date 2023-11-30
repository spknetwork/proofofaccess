package messaging

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"proofofaccess/localdata"
	"proofofaccess/pubsub"
)

func SendSyncing(req Request, ws *websocket.Conn) {
	localdata.Synced = true
	data := map[string]string{
		"type": "Syncing",
		"user": localdata.GetNodeName(),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error encoding JSON:", err)
		return
	}
	if localdata.WsPeers[req.User] == req.User && localdata.NodeType == 1 {
		WsMutex.Lock()
		ws.WriteMessage(websocket.TextMessage, jsonData)
		WsMutex.Unlock()
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		WsMutex.Lock()
		ws.WriteMessage(websocket.TextMessage, jsonData)
		WsMutex.Unlock()
	} else {
		pubsub.Publish(string(jsonData), req.User)
	}
}
func SendSynced(req Request, ws *websocket.Conn) {
	data := map[string]string{
		"type": "Synced",
		"user": localdata.GetNodeName(),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error encoding JSON:", err)
		return
	}
	if localdata.WsPeers[req.User] == req.User && localdata.NodeType == 1 {
		fmt.Println("Sending Synced to " + req.User)
		WsMutex.Lock()
		ws.WriteMessage(websocket.TextMessage, jsonData)
		WsMutex.Unlock()
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		WsMutex.Lock()
		localdata.WsValidators[req.User].WriteMessage(websocket.TextMessage, jsonData)
		WsMutex.Unlock()
	} else {
		pubsub.Publish(string(jsonData), req.User)
	}

}
func ReceiveSyncing(req Request) {
	localdata.Lock.Lock()
	localdata.ValidatorsStatus[req.User] = "Syncing"
	localdata.Lock.Unlock()
	fmt.Println("Syncing with " + req.User)
}
func ReceiveSynced(req Request) {
	localdata.Lock.Lock()
	localdata.ValidatorNames = localdata.RemoveDuplicates(append(localdata.ValidatorNames, req.User))
	localdata.ValidatorsStatus[req.User] = "Synced"
	localdata.Lock.Unlock()
	fmt.Println("Synced with " + req.User)
}
func SyncNode(req Request, ws *websocket.Conn) {
	fmt.Println("Syncing with " + req.User)
	peerName := req.User
	localdata.Lock.Lock()
	Nodes[req.User] = true
	localdata.PeerNames = localdata.RemoveDuplicates(append(localdata.PeerNames, peerName))
	localdata.NodesStatus[req.User] = "Synced"
	localdata.Lock.Unlock()
	SendSynced(req, ws)
	fmt.Println("Synced with " + req.User)
	Nodes[req.User] = false
}
