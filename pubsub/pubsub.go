package pubsub

import (
	"context"
	"fmt"
	ipfs "github.com/ipfs/go-ipfs-api"
	"github.com/sirupsen/logrus"
	poaipfs "proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"time"
)

var (
	log = logrus.New()
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
	fmt.Println("Publishing message:", message, user)
	err := poaipfs.Shell.PubSubPublish(user, message)
	if err != nil {
		fmt.Println("Error publishing message:", err)
		return err
	}
	return nil
}
func PubsubHandler(ctx context.Context) {
	if poaipfs.Shell != nil {
		sub, err := Subscribe(localdata.NodeName)
		if err != nil {
			log.Error("Error subscribing to pubsub: ", err)
			return
		}

		log.Info("User:", localdata.NodeName)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := Read(sub)
				if err != nil {
					log.Error("Error reading from pubsub: ", err)
					continue
				}
				messaging.HandleMessage(msg)
			}
		}
	} else {
		time.Sleep(1 * time.Second)
	}
}
