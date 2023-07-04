package messaging

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"proofofaccess/database"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/proofcrypto"
	"proofofaccess/pubsub"
	"proofofaccess/validation"
	"strconv"
	"strings"
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

// SendProof
// This is the function that sends the proof of access to the validation node
func SendProof(req Request, hash string, seed string, user string) {
	data := map[string]string{
		"type": TypeProofOfAccess,
		"hash": hash,
		"seed": seed,
		"user": user,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error encoding JSON:", err)
		return
	}
	if localdata.WsPeers[req.User] == req.User && localdata.NodeType == 1 {
		wsMutex.Lock()
		ws := localdata.WsClients[req.User]
		ws.WriteMessage(websocket.TextMessage, jsonData)
		wsMutex.Unlock()
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		localdata.WsValidators["Validator1"].WriteMessage(websocket.TextMessage, jsonData)
	} else {
		pubsub.Publish(string(jsonData), user)
	}
}

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
	fmt.Println("Message received:", message)
	//Handle proof of access response from storage node
	if localdata.NodeType == 1 {
		if req.Type == TypeProofOfAccess {
			go HandleProofOfAccess(req)
		}

	}

	//Handle request for proof request from validation node
	if localdata.NodeType == 2 {
		if req.Type == TypeRequestProof {
			go HandleRequestProof(req)
		}
		if req.Type == TypePingPongPong {
			validatorName := req.User
			localdata.Validators[validatorName] = true
			fmt.Println("Validator", validatorName, "is online")
			fmt.Println(validatorName, ": ", localdata.Validators[validatorName])
		}
	}
	if req.Type == TypePingPongPong {
		Ping[req.Hash] = true
		fmt.Println("PingPongPong received")
	}
	if req.Type == TypePingPongPing {
		fmt.Println("PingPongPing received")
		PingPongPong(req, req.Hash, req.User)
		if localdata.NodeType == 1 && !Nodes[req.User] && localdata.NodesStatus[req.User] != "Synced" {
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
	fmt.Println("Message handled")

}

// HandleRequestProof
// This is the function that handles the request for proof from the validation node
func HandleRequestProof(req Request) {
	CID := req.CID
	hash := req.Hash
	if ipfs.IsPinned(CID) == true {
		validationHash := validation.CreatProofHash(hash, CID)
		SendProof(req, validationHash, hash, localdata.NodeName)
	} else {
		SendProof(req, hash, req.Seed, localdata.NodeName)
	}

}

// HandleProofOfAccess
// This is the function that handles the proof of access response from the storage node
func HandleProofOfAccess(req Request) {
	// Get the start time from the seed
	start := localdata.GetTime(req.Seed)
	fmt.Println("Start time:", start)
	// Get the current time
	elapsed := time.Since(start)
	fmt.Println("Elapsed time:", elapsed)
	// Set the elapsed time
	localdata.SetElapsed(req.Seed, elapsed)

	// Get the CID and Seed
	data := database.Read([]byte("Stats" + req.Seed))
	var message Request
	err := json.Unmarshal([]byte(string(data)), &message)
	if err != nil {
		fmt.Println("Error decoding JSON:", err)
	}
	seed := message.Seed
	CID := localdata.GetStatus(seed).CID
	ProofRequest[seed] = true
	// Create the proof hash
	var validationHash string
	if req.Hash != "" {
		fmt.Println("Creating proof of access hash")
		validationHash = validation.CreatProofHash(seed, CID)
		// Check if the proof of access is valid
		if validationHash == req.Hash && elapsed < 2500*time.Millisecond {
			fmt.Println("Proof of access is valid")
			fmt.Println(req.Seed)
			localdata.SetStatus(req.Seed, CID, "Valid", req.User)
		} else {
			fmt.Println("Request Hash", req.Hash)
			fmt.Println("Validation Hash", validationHash)
			fmt.Println("Elapsed time:", elapsed)
			fmt.Println("Proof of access is invalid took too long")
			localdata.SetStatus(req.Seed, CID, "Invalid", req.User)
		}
	} else {
		fmt.Println("Proof is invalid")
		localdata.SetStatus(req.Seed, CID, "Invalid", req.User)
	}
	ProofRequestStatus[seed] = true
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
		localdata.WsValidators["Validator1"].WriteMessage(websocket.TextMessage, jsonData)
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
		ws.WriteMessage(websocket.TextMessage, jsonData)
		wsMutex.Unlock()
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		localdata.WsValidators["Validator1"].WriteMessage(websocket.TextMessage, jsonData)
	} else {
		pubsub.Publish(string(jsonData), user)
	}
}
func RequestCIDS(req Request) {
	data := map[string]string{
		"type": "RequestCIDS",
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
		ws.WriteMessage(websocket.TextMessage, jsonData)
		wsMutex.Unlock()
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		localdata.WsValidators["Validator1"].WriteMessage(websocket.TextMessage, jsonData)
	} else {
		pubsub.Publish(string(jsonData), req.User)
	}

}
func SendCIDS(name string) {
	allPins, _ := ipfs.Shell.Pins()

	NewPins := make([]string, 0)
	for key, pinInfo := range allPins {
		if pinInfo.Type == "recursive" {
			NewPins = append(NewPins, key)
		}
	}
	pinsJson, err := json.Marshal(NewPins)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Split the pinsJson into smaller chunks
	chunks := splitIntoChunks(string(pinsJson), 3000) // 1000 is the chunk size, adjust as needed
	seed, _ := proofcrypto.CreateRandomHash()
	for i, chunk := range chunks {
		data := map[string]string{
			"type":       "SendCIDS",
			"user":       localdata.GetNodeName(),
			"seed":       seed,
			"pins":       chunk,
			"part":       strconv.Itoa(i + 1),
			"totalParts": strconv.Itoa(len(chunks)),
		}

		jsonData, err := json.Marshal(data)
		if err != nil {
			fmt.Println("Error encoding JSON:", err)
			return
		}

		if localdata.UseWS == true && localdata.NodeType == 2 {
			wsMutex.Lock()
			ws := localdata.WsValidators["Validator1"]
			ws.WriteJSON(data)
			wsMutex.Unlock()
		} else {
			pubsub.Publish(string(jsonData), name)
		}
	}
}

// splitIntoChunks splits a string into chunks of the specified size
func splitIntoChunks(s string, chunkSize int) []string {
	var chunks []string
	runes := []rune(s)

	for i := 0; i < len(runes); {
		end := i + chunkSize

		if end > len(runes) {
			end = len(runes)
		}

		// Check if the end index is in the middle of a CID
		if end < len(runes) && runes[end] != ',' {
			for end < len(runes) && runes[end] != ',' {
				end++
			}
		}

		// Remove leading and trailing commas, and enclose chunk in brackets
		chunk := string(runes[i:end])
		chunk = strings.Trim(chunk, ",")
		chunk = strings.Trim(chunk, "[")
		chunk = strings.Trim(chunk, "]")
		chunk = "[" + chunk + "]"

		chunks = append(chunks, chunk)

		// Move the start index to the next CID
		i = end
		if i < len(runes) && runes[i] == ',' {
			i++
		}
	}

	return chunks
}

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
		wsMutex.Lock()
		ws := localdata.WsClients[req.User]
		ws.WriteMessage(websocket.TextMessage, jsonData)
		wsMutex.Unlock()
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		localdata.WsValidators["Validator1"].WriteMessage(websocket.TextMessage, jsonData)
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
		wsMutex.Lock()
		ws := localdata.WsClients[req.User]
		ws.WriteMessage(websocket.TextMessage, jsonData)
		wsMutex.Unlock()
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		localdata.WsValidators["Validator1"].WriteMessage(websocket.TextMessage, jsonData)
	} else {
		pubsub.Publish(string(jsonData), req.User)
	}

}
func ReceiveSyncing(req Request) {
	localdata.ValidatorsStatus[req.User] = "Syncing"
	fmt.Println("Syncing with " + req.User)
}
func ReceiveSynced(req Request) {
	localdata.ValidatorNames = localdata.RemoveDuplicates(append(localdata.ValidatorNames, req.User))
	localdata.ValidatorsStatus[req.User] = "Synced"
	fmt.Println("Synced with " + req.User)
}
func SyncNode(req Request) {
	fmt.Println("Syncing with " + req.User)
	Nodes[req.User] = true
	peerName := req.User
	localdata.PeerNames = localdata.RemoveDuplicates(append(localdata.PeerNames, peerName))
	localdata.NodesStatus[req.User] = "Syncing"
	fmt.Println(req.Pins)
	var myData map[string]interface{}

	if localdata.WsPeers[req.User] != req.User && localdata.NodeType == 1 {
		err := ipfs.DownloadAndDecodeJSON(req.CID, &myData)
		if err != nil {
			Nodes[req.User] = false
			log.Println(err)
			time.Sleep(5 * time.Second)
			RequestCIDS(req)
			return
		}
	}

	// Handle the chunks of allPins data
	part, _ := strconv.Atoi(req.Part)
	totalParts, _ := strconv.Atoi(req.TotalParts)

	var pins []string
	err := json.Unmarshal([]byte(req.Pins), &pins)
	if err != nil {
		log.Println("Error unmarshalling pins:", err)
		log.Println("Pins data:", req.Pins)
		return
	}

	allPins := localdata.PeerCids[req.User]
	for _, value := range pins {
		allPins = append(allPins, value)
	}
	localdata.PeerCids[req.User] = allPins
	localdata.PeerSyncSeed[req.Seed] = localdata.PeerSyncSeed[req.Seed] + 1
	fmt.Println("Received", len(pins), "CIDs from", req.User, "(", part, "/", totalParts, ")")
	// If this is the last chunk, sync the node
	fmt.Println("Checking if all parts received")
	fmt.Println(localdata.PeerSyncSeed[req.Seed])
	// Convert allPins to JSON
	allPinsJson, err := json.Marshal(allPins)
	if err != nil {
		log.Println("Error marshalling allPins:", err)
		return
	}

	// Save allPins to the database
	database.Save([]byte("allPins:"+req.User), allPinsJson)
	if localdata.PeerSyncSeed[req.Seed] == totalParts {
		allPinsMap := make(map[string]interface{})
		for _, pin := range allPins {
			allPinsMap[pin] = nil
		}

		go ipfs.SyncNode(allPinsMap, req.User)
	} else {
		return
	}

	go func() {
		for {
			time.Sleep(1 * time.Second)
			fmt.Println("Checking if synced with " + req.User)
			fmt.Println(localdata.NodesStatus[req.User])
			if localdata.NodesStatus[req.User] == "Synced" {
				println("test if", localdata.NodesStatus[req.User])
				fmt.Println("Synced with " + req.User)
				SendSynced(req)
				Nodes[req.User] = false
				break
			} else if localdata.NodesStatus[req.User] == "Failed" {
				RequestCIDS(req)
				break
			}
		}
	}()
	SendSyncing(req)
}
