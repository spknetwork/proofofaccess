package messaging

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"proofofaccess/database"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/pubsub"
	"strconv"
	"time"
)

func SendSyncing(req Request) {
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
		ws := localdata.WsClients[req.User]
		WsMutex.Lock()
		ws.WriteMessage(websocket.TextMessage, jsonData)
		WsMutex.Unlock()
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		ws := localdata.WsClients[req.User]
		WsMutex.Lock()
		ws.WriteMessage(websocket.TextMessage, jsonData)
		WsMutex.Unlock()
	} else {
		pubsub.Publish(string(jsonData), req.User)
	}
}
func SendSynced(req Request) {
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
		ws := localdata.WsClients[req.User]
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
func SyncNode(req Request) {
	fmt.Println("Syncing with " + req.User)
	localdata.Lock.Lock()
	Nodes[req.User] = true
	localdata.Lock.Unlock()
	fmt.Println("Requesting CIDS")
	peerName := req.User
	localdata.Lock.Lock()
	localdata.PeerNames = localdata.RemoveDuplicates(append(localdata.PeerNames, peerName))
	localdata.NodesStatus[req.User] = "Syncing"
	localdata.Lock.Unlock()
	fmt.Println("Requesting CIDS from " + req.User)
	fmt.Println(req.Pins)
	var myData map[string]interface{}

	if localdata.WsPeers[req.User] != req.User && localdata.NodeType == 1 {
		fmt.Println("Ipfs DownloadAndDecodeJSON")
		err := ipfs.DownloadAndDecodeJSON(req.CID, &myData)
		if err != nil {
			localdata.Lock.Lock()
			Nodes[req.User] = false
			localdata.Lock.Unlock()
			log.Println(err)
			time.Sleep(5 * time.Second)
			RequestCIDS(req)
			return
		}
	}

	// Handle the chunks of allPins data
	part, _ := strconv.Atoi(req.Part)
	totalParts, _ := strconv.Atoi(req.TotalParts)
	fmt.Println("Part", part, "of", totalParts)
	var pins []string
	err := json.Unmarshal([]byte(req.Pins), &pins)
	if err != nil {
		log.Println("Error unmarshalling pins:", err)
		log.Println("Pins data:", req.Pins)
		return
	}
	// Lock to safely read from shared data
	localdata.Lock.Lock()
	allPins := localdata.PeerCids[req.User]
	localdata.Lock.Unlock()

	// Create a map to use as a set for unique values
	uniquePins := make(map[string]struct{})

	// Populate the map with the existing pins
	for _, pin := range allPins {
		uniquePins[pin] = struct{}{}
	}

	// Add new pins to the map, automatically removing duplicates
	for _, pin := range pins {
		uniquePins[pin] = struct{}{}
	}

	// Convert the map keys back into a slice
	allPins = make([]string, 0, len(uniquePins))
	for pin := range uniquePins {
		allPins = append(allPins, pin)
	}

	// Lock to safely write to shared data
	localdata.Lock.Lock()
	localdata.PeerCids[req.User] = allPins
	localdata.PeerSyncSeed[req.Seed] = localdata.PeerSyncSeed[req.Seed] + 1
	localdata.Lock.Unlock()
	fmt.Println("Received", len(pins), "CIDs from", req.User, "(", part, "/", totalParts, ")")
	// If this is the last chunk, sync the node
	// Convert allPins to JSON
	allPinsJson, err := json.Marshal(allPins)
	if err != nil {
		log.Println("Error marshalling allPins:", err)
		return
	}

	// Save allPins to the database
	database.Save([]byte("allPins:"+req.User), allPinsJson)
	fmt.Println("Saved allPins to database")
	syncSeed := localdata.PeerSyncSeed[req.Seed]
	fmt.Println("Sync seed:", syncSeed)
	fmt.Println("Total parts:", totalParts)
	if syncSeed == totalParts {
		// Calculate the size of the pins
		var size int
		for _, pin := range allPins {
			size = localdata.CidSize[pin] + size
		}
		localdata.Lock.Lock()
		localdata.PeerSize[req.User] = size
		localdata.NodesStatus[req.User] = "Synced"
		localdata.Lock.Unlock()
	} else if syncSeed == 1 {
		SendSyncing(req)
	} else {
		return
	}

	go func() {
		for {
			time.Sleep(1 * time.Second)
			fmt.Println("Checking if synced with " + req.User)
			localdata.Lock.Lock()
			nodeStatus := localdata.NodesStatus[req.User]
			localdata.Lock.Unlock()
			if nodeStatus == "Synced" {
				fmt.Println("Synced with " + req.User)
				SendSynced(req)
				localdata.Lock.Lock()
				Nodes[req.User] = false
				localdata.Lock.Unlock()
				break
			} else if nodeStatus == "Failed" {
				RequestCIDS(req)
				break
			}
		}
	}()
}
