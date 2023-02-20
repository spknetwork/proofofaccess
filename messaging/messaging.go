package messaging

import (
	"encoding/json"
	"fmt"
	"proofofaccess/localdata"
	"proofofaccess/pubsub"
	"proofofaccess/validation"
	"strings"
	"time"
)

type Request struct {
	Type string `json:"type"`
	Hash string `json:"hash"`
	CID  string `json:"CID"`
	Seed string `json:"seed"`
}

var request Request

// SendProof
// This is the function that sends the proof of access to the validation node
func SendProof(hash string, seed string) {
	jsonString := `{"type": "ProofOfAccess", "hash":"` + hash + `", "seed":"` + seed + `"}`
	jsonString = strings.TrimSpace(jsonString)
	pubsub.Publish(jsonString)
}

// HandleMessage
// This is the function that handles the messages from the pubsub
func HandleMessage(message string, nodeType *int) {
	// JSON decode message
	err := json.Unmarshal([]byte(message), &request)
	if err != nil {
		fmt.Println("Error decoding JSON:", err)
	}

	//Handle proof of access response from storage node
	if *nodeType == 1 {
		if request.Type == "ProofOfAccess" {
			HandleProofOfAccess(request)
		}
	}

	//Handle request for proof request from validation node
	if *nodeType == 2 {
		if request.Type == "RequestProof" {
			HandleRequestProof(request)
		}
	}
}

// HandleRequestProof
// This is the function that handles the request for proof from the validation node
func HandleRequestProof(request Request) {
	CID := request.CID
	hash := request.Hash
	validationHash := validation.CreatProofHash(hash, CID)
	SendProof(validationHash, hash)
}

// HandleProofOfAccess
// This is the function that handles the proof of access response from the storage node
func HandleProofOfAccess(request Request) {
	// Get the start time from the seed
	start := localdata.GetTime(request.Seed)
	fmt.Println("Start time:", start)

	// Get the current time
	elapsed := time.Since(start)

	// Set the elapsed time
	localdata.SetElapsed(request.Seed, elapsed)

	// Get the CID and Seed
	CID := request.CID
	Seed := request.Seed

	// Create the proof hash
	validationHash := validation.CreatProofHash(Seed, CID)

	// Check if the proof of access is valid
	if validationHash == request.Hash && elapsed < 150*time.Millisecond {
		fmt.Println("Proof of access is valid")
		fmt.Println(request.Seed)
		localdata.SetStatus(request.Seed, CID, "Valid")
	} else {
		localdata.SetStatus(request.Seed, CID, "Invalid")
		fmt.Println("Proof of access is invalid took too long")
	}
}
