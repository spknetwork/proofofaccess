package messaging

import (
	"encoding/json"
	"proofofaccess/localdata"
	"proofofaccess/pubsub"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

func SendSyncing(req Request, ws *websocket.Conn) {
	localdata.Synced = true
	data := map[string]string{
		"type": "Syncing",
		"user": localdata.GetNodeName(),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		logrus.Errorf("Error encoding Syncing JSON: %v", err)
		return
	}
	if localdata.WsPeers[req.User] == req.User && localdata.NodeType == 1 {
		WsMutex.Lock()
		err = ws.WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			logrus.Errorf("Error writing Syncing message to WebSocket for %s: %v", req.User, err)
		}
		WsMutex.Unlock()
	} else if localdata.UseWS && localdata.NodeType == 2 {
		WsMutex.Lock()
		err = ws.WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			logrus.Errorf("Error writing Syncing message to WebSocket validator for %s: %v", req.User, err)
		}
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
		logrus.Errorf("Error encoding Synced JSON: %v", err)
		return
	}
	if localdata.WsPeers[req.User] == req.User && localdata.NodeType == 1 {
		if ws == nil {
			logrus.Debugf("SendSynced request for peer %s via PubSub (no WebSocket), publishing response", req.User)
			pubsub.Publish(string(jsonData), req.User)
			return
		}
		WsMutex.Lock()
		err = ws.WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			logrus.Errorf("Error writing Synced message to WebSocket for %s: %v", req.User, err)
		}
		WsMutex.Unlock()
	} else if localdata.UseWS && localdata.NodeType == 2 {
		WsMutex.Lock()
		err = localdata.WsValidators[req.User].WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			logrus.Errorf("Error writing Synced message to WebSocket validator %s: %v", req.User, err)
			localdata.Lock.Lock()
			delete(localdata.WsValidators, req.User)
			localdata.Lock.Unlock()
		}
		WsMutex.Unlock()
	} else {
		pubsub.Publish(string(jsonData), req.User)
	}

}
func ReceiveSyncing(req Request) {
	localdata.Lock.Lock()
	localdata.ValidatorsStatus[req.User] = "Syncing"
	localdata.Lock.Unlock()
}
func ReceiveSynced(req Request) {
	localdata.Lock.Lock()
	localdata.ValidatorNames = localdata.RemoveDuplicates(append(localdata.ValidatorNames, req.User))
	localdata.ValidatorsStatus[req.User] = "Synced"
	localdata.Lock.Unlock()
}
func SyncNode(req Request, ws *websocket.Conn) {
	peerName := req.User
	localdata.Lock.Lock()
	Nodes[req.User] = true
	localdata.PeerNames = localdata.RemoveDuplicates(append(localdata.PeerNames, peerName))
	localdata.NodesStatus[req.User] = "Synced"
	localdata.Lock.Unlock()
	SendSynced(req, ws)
	Nodes[req.User] = false
}
