package api

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"math"
	"proofofaccess/localdata"
	"strings"
)

func stats(c *websocket.Conn) {
	NetworkStorage := 0
	localdata.Lock.Lock()
	if localdata.NodeType == 2 {
		NetworkStorage = localdata.PeerSize[localdata.NodeName]
	} else {
		for _, peerName := range localdata.PeerNames {
			fmt.Println("Peer: ", peerName)
			fmt.Println("Size: ", localdata.PeerSize[peerName])
			NetworkStorage = NetworkStorage + localdata.PeerSize[peerName]
		}
	}
	peerSizes := make(map[string]string) // Create a new map to hold each peer's size

	for peerName, size := range localdata.PeerSize {
		peerSizes[peerName] = fmt.Sprintf("%d", size)
	}
	fmt.Println("Network Storage: ", NetworkStorage)
	// Print the Network Storage in GB
	NetworkStorage = NetworkStorage / 1024 / 1024 / 1024
	fmt.Println("Size: ", NetworkStorage, "GB")
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
			"NetworkStorage":   fmt.Sprintf("%d GB", NetworkStorage),
			"SyncedPercentage": fmt.Sprintf("%f", math.Round(float64(localdata.SyncedPercentage))),
		},
		"PeerSizes":      peerSizes,
		"PeerLastActive": localdata.PeerLastActive,
	}
	localdata.Lock.Unlock()
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	c.WriteMessage(websocket.TextMessage, jsonData)

}
