package messaging

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"proofofaccess/database"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/pubsub"
	"proofofaccess/validation"
	"time"
)

// HandleRequestProof
// This is the function that handles the request for proof from the validation node
func HandleRequestProof(req Request) {
	CID := req.CID
	hash := req.Hash
	if ipfs.IsPinnedInDB(CID) == true {
		fmt.Println("Sending proof of access to validation node")
		validationHash := validation.CreatProofHash(hash, CID)
		SendProof(req, validationHash, hash, localdata.NodeName)
	} else {
		fmt.Println("Pin not found")
		SendProof(req, "NA", hash, localdata.NodeName)
	}
}

// HandleProofOfAccess
// This is the function that handles the proof of access response from the storage node
func HandleProofOfAccess(req Request) {
	fmt.Println("Handling proof of access response from storage node")
	// Get the start time from the seed
	start := localdata.GetTime(req.Seed)
	fmt.Println("Start time:", start)
	// Get the current time
	elapsed := time.Since(start)
	// fmt.Println("Elapsed time:", elapsed)
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
	localdata.Lock.Lock()
	ProofRequest[seed] = true
	localdata.Lock.Unlock()
	// Create the proof hash
	var validationHash string
	fmt.Println("Request Hash", req.Hash)
	if req.Hash != "" || req.Hash != "NA" {
		fmt.Println("Creating proof of access hash")
		validationHash = validation.CreatProofHash(seed, CID)
		fmt.Println("Validation Hash", validationHash)
		// Check if the proof of access is valid
		if validationHash == req.Hash && elapsed < 25000000*time.Millisecond {
			fmt.Println("Proof of access is valid")
			//fmt.Println(req.Seed)
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
	localdata.Lock.Lock()
	ProofRequestStatus[seed] = true
	localdata.Lock.Unlock()
}

// SendProof
// This is the function that sends the proof of access to the validation node
func SendProof(req Request, hash string, seed string, user string) {
	fmt.Println("Sending proof of access to validation node")
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
	wsPeers := localdata.WsPeers[req.User]
	nodeType := localdata.NodeType
	if wsPeers == req.User && nodeType == 1 {
		ws := localdata.WsClients[req.User]
		WsMutex.Lock()
		ws.WriteMessage(websocket.TextMessage, jsonData)
		WsMutex.Unlock()
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		ws := localdata.WsValidators[req.User]
		WsMutex.Lock()
		ws.WriteMessage(websocket.TextMessage, jsonData)
		WsMutex.Unlock()
		fmt.Println("Sent proof of access to validation node")
	} else {
		pubsub.Publish(string(jsonData), user)
	}
}
