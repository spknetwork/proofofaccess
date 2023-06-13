package api

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"proofofaccess/localdata"
	"strings"
)

func stats(c *websocket.Conn) {
	NetworkStorage := 0
	fmt.Println(localdata.NodeType)
	if localdata.NodeType == 2 {
		NetworkStorage = localdata.PeerSize[localdata.NodeName]
	} else {
		for _, peerName := range localdata.PeerNames {
			fmt.Println("Peer: ", peerName)
			fmt.Println("Size: ", localdata.PeerSize[peerName])
			NetworkStorage = NetworkStorage + localdata.PeerSize[peerName]
		}
	}

	fmt.Println("Network Storage: ", NetworkStorage)
	// Print the Network Storage in GB
	NetworkStorage = NetworkStorage / 1024 / 1024
	fmt.Println("Size: ", NetworkStorage, "MB")
	fmt.Println("NodeType: ", localdata.NodeType)
	NodeType := ""
	if localdata.NodeType == 1 {
		NodeType = "Validator"
	} else {
		NodeType = "Storage"
	}
	data := map[string]interface{}{
		"Status": map[string]string{
			"Sync":             fmt.Sprintf("%v", localdata.Synced),
			"PeersCount":       fmt.Sprintf("%d", len(localdata.PeerNames)),
			"ValidatorCount":   fmt.Sprintf("%d", len(localdata.ValidatorNames)),
			"Node":             fmt.Sprintf(localdata.NodeName),
			"Type":             fmt.Sprintf(NodeType),
			"Peers":            strings.Join(localdata.PeerNames, ","),
			"Validators":       strings.Join(localdata.ValidatorNames, ","),
			"NetworkStorage":   fmt.Sprintf("%d MB", NetworkStorage),
			"SyncedPercentage": fmt.Sprintf("%f", localdata.SyncedPercentage),
		},
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	c.WriteMessage(websocket.TextMessage, jsonData)

}
