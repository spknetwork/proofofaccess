package pubsub

import (
	"context"
	"fmt"
	ipfs "github.com/ipfs/go-ipfs-api"
)

var Shell = ipfs.NewLocalShell()

// Subscribe to a topic
func Subscribe() (*ipfs.PubSubSubscription, error) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub, err := Shell.PubSubSubscribe("example-topic")
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
	return string(msg.Data), nil
}

// Publish a message to a topic
func Publish(message string) error {
	err := Shell.PubSubPublish("example-topic", message)
	if err != nil {
		fmt.Println("Error publishing message:", err)
		return err
	}
	return nil
}
