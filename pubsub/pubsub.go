package pubsub

import (
	"context"
	poaipfs "proofofaccess/ipfs"

	iface "github.com/ipfs/kubo/core/coreiface"
	"github.com/sirupsen/logrus"
)

// Subscribe to a topic
func Subscribe(username string) (iface.PubSubSubscription, error) {
	sub, err := poaipfs.Shell.PubSubSubscribe(username)
	if err != nil {
		logrus.Errorf("Error subscribing to PubSub topic %s: %v", username, err)
		return nil, err
	}
	return sub, nil
}

// Read the message from the subscription
func Read(sub iface.PubSubSubscription) (string, error) {
	msg, err := sub.Next(context.Background())
	if err != nil {
		logrus.Errorf("Error receiving PubSub message: %v", err)
		return "", err
	}
	logrus.Debugf("Received PubSub message from %s", msg.From().String())
	return string(msg.Data()), nil
}

// Publish a message to a topic
func Publish(message string, user string) error {
	err := poaipfs.Shell.PubSubPublish(user, message)
	if err != nil {
		logrus.Errorf("Error publishing PubSub message to %s: %v", user, err)
		return err
	}
	logrus.Debugf("Published PubSub message to %s", user)
	return nil
}
