package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"proofofaccess/messaging"
	"proofofaccess/proofcrypto"
	"proofofaccess/pubsub"
	"proofofaccess/validation"
)

// Declare a variable to store the node type
var nodeType = flag.Int("node", 1, "Node type 1 = validation 2 = access")
var CID = ""
var Hash = ""

func main() {
	// Parse the command line flags
	flag.Parse()

	// Subscribe to a topic
	sub, _ := pubsub.Subscribe()
	go func() {
		for {
			// Read the message from the subscription
			msg, _ := pubsub.Read(sub)
			fmt.Println("Received message:", msg)
			// Handle the message

			proofHash, requestType := messaging.HandleMessage(msg, nodeType)

			if *nodeType == 1 && requestType == "ProofOfAccess" {
				validationHash := validation.CreatProofHash(Hash, CID)
				fmt.Println("Proof of access hash:", proofHash)
				fmt.Println("Validation hash:", validationHash)
				fmt.Println("CID:", CID)
				fmt.Println("Hash:", Hash)
				if validationHash == proofHash {
					fmt.Println("Proof of access is valid")
				} else {
					fmt.Println("Proof of access is invalid")
				}
			}
		}
	}()
	for {
		// Prompt the user to enter a message to send
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter message to send: ")
		CID, _ = reader.ReadString('\n')
		// Publish the message to the topic
		Hash = proofcrypto.CreateRandomHash()
		json, _ := validation.ProofRequestJson(Hash, CID)
		fmt.Println("Sending message:", json)
		pubsub.Publish(json)
	}
}
