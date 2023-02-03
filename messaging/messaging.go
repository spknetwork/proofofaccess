package messaging

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"proofofaccess/pubsub"
	"proofofaccess/validation"
	"strings"
)

func SendMessage(conn net.Conn, message string) error {
	_, err := fmt.Fprintf(conn, message)
	if err != nil {
		return fmt.Errorf("Error sending message: %v", err)
	}
	return nil
}

func SendProof(hash string) {
	jsonString := `{"type": "ProofOfAccess", "hash":"` + hash + `"}`
	jsonString = strings.TrimSpace(jsonString)
	pubsub.Publish(jsonString)

}

type Request struct {
	Type string `json:"type"`
	Hash string `json:"hash"`
	CID  string `json:"CID"`
}

//func ReceiveMessage(conn net.Conn, scanner *bufio.Scanner) {
//	for scanner.Scan() {
//		var request Request
//		err := json.Unmarshal([]byte(scanner.Text()), &request)
//		if err != nil {
//			fmt.Println("Error decoding JSON:", err)
//			return
//		}
//		fmt.Println(scanner.Text())
//		fmt.Println("Type:", request.Type)
//		fmt.Println("Hash:", request.Hash)
//		fmt.Println("CID:", request.CID)
//
//		if request.Type == "RequestProof" {
//			CID := request.CID
//			hash := request.Hash
//			validationHash := validation.CreatProofHash(hash, CID)
//			SendProof(validationHash)
//
//		}
//	}
//}

func HandleMessage(message string, nodeType *int) (string, string) {
	var request Request
	err := json.Unmarshal([]byte(message), &request)
	if err != nil {
		fmt.Println("Error decoding JSON:", err)
		return "", ""
	}
	//fmt.Println("Type:", request.Type)
	//fmt.Println("Hash:", request.Hash)
	//fmt.Println("CID:", request.CID)

	if request.Type == "RequestProof" && *nodeType == 2 {
		CID := request.CID
		hash := request.Hash
		validationHash := validation.CreatProofHash(hash, CID)
		SendProof(validationHash)
		return validationHash, request.Type
	}
	if request.Type == "ProofOfAccess" && *nodeType == 1 {
		hash := request.Hash
		return hash, request.Type
	}
	return "", ""
}

func CliMessage() string {
	fmt.Print("Enter message to send: ")
	input := bufio.NewReader(os.Stdin)
	text, _ := input.ReadString('\n')
	text = strings.TrimSpace(text)
	return text
}
