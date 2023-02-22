package main

import (
	"flag"
	"fmt"
	"proofofaccess/api"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"proofofaccess/pubsub"
)

// Declare a variable to store the node type
var nodeType = flag.Int("node", 1, "Node type 1 = validation 2 = access")
var username = flag.String("username", "", "Node type 1 = validation 2 = access")

var CID = ""
var Hash = ""

// Main function
func main() {
	// Parse the command line flags
	flag.Parse()
	localdata.SetNodeName(*username)
	// Start the API
	go func() {
		if *nodeType == 1 {
			api.Api()
		}
	}()
	// Subscribe to pubsub channel
	sub, _ := pubsub.Subscribe(*username)
	fmt.Println("User:", *username)
	go func() {
		for {
			// Read the message from pubsub
			msg, _ := pubsub.Read(sub)
			// Handle the message
			messaging.HandleMessage(msg, nodeType)
		}
	}()
	for {
		if *nodeType == 1 {
			messaging.PingPong()
		}
	}
}
