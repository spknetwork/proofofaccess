package pubsub

import (
	"context"
	"fmt"
	poaipfs "proofofaccess/ipfs"

	ipfs "github.com/ipfs/go-ipfs-api"
)

// Subscribe to a topic
func Subscribe(username string) (*ipfs.PubSubSubscription, error) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub, err := poaipfs.Shell.PubSubSubscribe(username)
	if err != nil {
		fmt.Println("Error subscribing:", err)
		return nil, err
	}
	return sub, nil
}

// Read the message from the subscription
func Read(sub *ipfs.PubSubSubscription) (string, error) {
	msg, err := sub.Next()
	if err != nil {
		fmt.Println("Error receiving message:", err)
		return "", err
	}
	fmt.Println("Message from: ", msg.From.String())
	return string(msg.Data), nil
}

// Publish a message to a topic
func Publish(message string, user string) error {
	//fmt.Println("Publishing message:", message, user)
	err := poaipfs.Shell.PubSubPublish(user, message)
	if err != nil {
		//fmt.Println("Error publishing message:", err)
		return err
	}
	return nil
}
