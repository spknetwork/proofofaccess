package messaging

import (
	"encoding/json"
	"fmt"
	"proofofaccess/database"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/pubsub"
	"proofofaccess/validation"
	"time"
)

type Request struct {
	Type string `json:"type"`
	Hash string `json:"hash"`
	CID  string `json:"cid"`
	Seed string `json:"seed"`
	User string `json:"user"`
}

const (
	Layout            = "2006-01-02 15:04:05.999999 -0700 MST m=+0.000000000"
	TypeProofOfAccess = "ProofOfAccess"
	TypeRequestProof  = "RequestProof"
	TypePingPongPing  = "PingPongPing"
	TypePingPongPong  = "PingPongPong"
)

var request Request

var Ping = map[string]bool{}
var ProofRequest = map[string]bool{}
var ProofRequestStatus = map[string]bool{}

// SendProof
// This is the function that sends the proof of access to the validation node
func SendProof(hash string, seed string, user string) {
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

	pubsub.Publish(string(jsonData), user)
}

// HandleMessage
// This is the function that handles the messages from the pubsub
func HandleMessage(message string, nodeType *int) {
	// JSON decode message
	err := json.Unmarshal([]byte(message), &request)
	if err != nil {
		fmt.Println("Error decoding JSON:", err)
		return
	}
	fmt.Println("Message received:", message)
	//Handle proof of access response from storage node
	if *nodeType == 1 {
		if request.Type == TypeProofOfAccess {
			HandleProofOfAccess(request)
		}

	}

	//Handle request for proof request from validation node
	if *nodeType == 2 {
		if request.Type == TypeRequestProof {
			HandleRequestProof(request)
		}
		if request.Type == TypePingPongPong {
			validatorName := request.User
			localdata.Validators[validatorName] = true
			fmt.Println("Validator", validatorName, "is online")
			fmt.Println(validatorName, ": ", localdata.Validators[validatorName])
		}
	}
	if request.Type == TypePingPongPong {
		Ping[request.Hash] = true
		fmt.Println("PingPongPong received")
	}
	if request.Type == TypePingPongPing {
		fmt.Println("PingPongPing received")
		PingPongPong(request.Hash, localdata.NodeName)
	}
}

// HandleRequestProof
// This is the function that handles the request for proof from the validation node
func HandleRequestProof(request Request) {
	CID := request.CID
	hash := request.Hash
	user := request.User
	if ipfs.IsPinned(CID) == true {
		validationHash := validation.CreatProofHash(hash, CID)
		SendProof(validationHash, hash, user)
	} else {
		SendProof("", hash, user)
	}

}

// HandleProofOfAccess
// This is the function that handles the proof of access response from the storage node
func HandleProofOfAccess(request Request) {
	// Get the start time from the seed
	start := localdata.GetTime(request.Seed)

	// Get the current time
	elapsed := time.Since(start)

	// Set the elapsed time
	localdata.SetElapsed(request.Seed, elapsed)

	// Get the CID and Seed
	data := database.Read([]byte(request.Seed))
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
	if request.Hash != "" {
		validationHash = validation.CreatProofHash(seed, CID)
		// Check if the proof of access is valid
		if validationHash == request.Hash && elapsed < 2500*time.Millisecond {
			fmt.Println("Proof of access is valid")
			fmt.Println(request.Seed)
			localdata.SetStatus(request.Seed, CID, "Valid")
		} else {
			fmt.Println("Elapsed time:", elapsed)
			fmt.Println("Proof of access is invalid took too long")
			localdata.SetStatus(request.Seed, CID, "Invalid")
		}
	} else {
		fmt.Println("Proof is invalid")
		localdata.SetStatus(request.Seed, CID, "Invalid")
	}
	ProofRequestStatus[seed] = true
}

func PingPong(hash string, user string) {
	fmt.Println("PingPong")
	localdata.SaveTime(hash)
	fmt.Println("PingPongPing sent")
	PingPongPing(hash, user)
}

func PingPongPing(hash string, user string) {
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
	pubsub.Publish(string(jsonData), user)
}

func PingPongPong(hash string, user string) {
	data := map[string]string{
		"type": TypePingPongPong,
		"hash": hash,
		"user": user,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error encoding JSON:", err)
		return
	}
	pubsub.Publish(string(jsonData), user)
}
