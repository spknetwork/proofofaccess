package validators

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"proofofaccess/proofcrypto"
	"proofofaccess/pubsub"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type Service struct {
	Domain   string `json:"a"` // Domain
	NodeName string `json:"b"` // NodeName
}

type Services struct {
	Services []map[string]Service `json:"services"`
}

func GetValidators(validatorsUrl string) {
	logrus.Debugf("Fetching validators from: %s", validatorsUrl)
	resp, err := http.Get(validatorsUrl)
	if err != nil {
		logrus.Errorf("Error fetching validators from %s: %v", validatorsUrl, err)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logrus.Errorf("Error reading validator response body from %s: %v", validatorsUrl, err)
		return
	}

	var services Services
	err = json.Unmarshal(body, &services)
	if err != nil {
		logrus.Errorf("Error unmarshalling validator JSON from %s: %v", validatorsUrl, err)
		return
	}

	var loadedValidators []string // Collect data to log outside lock
	localdata.Lock.Lock()
	// defer localdata.Lock.Unlock() // Unlock manually before logging
	for _, serviceMap := range services.Services {
		for _, service := range serviceMap {
			if strings.HasPrefix(service.Domain, "https://") {
				service.Domain = "wss://" + strings.TrimPrefix(service.Domain, "https://")
			}
			if strings.HasSuffix(service.Domain, "/") {
				service.Domain = strings.TrimSuffix(service.Domain, "/")
			}
			localdata.ValidatorNames = append(localdata.ValidatorNames, service.NodeName)
			localdata.ValidatorAddress[service.NodeName] = service.Domain
			// logrus.Debugf("Loaded validator: %s (%s)", service.NodeName, service.Domain) // Log outside lock
			loadedValidators = append(loadedValidators, fmt.Sprintf("%s (%s)", service.NodeName, service.Domain)) // Collect info
		}
	}
	// Deduplicate before releasing lock
	localdata.ValidatorNames = localdata.RemoveDuplicates(localdata.ValidatorNames)
	localdata.Lock.Unlock()

	// Log collected info outside the lock
	if len(loadedValidators) > 0 {
		logrus.Debugf("Loaded validators: %s", strings.Join(loadedValidators, ", "))
	}
}

func ConnectToValidators(ctx context.Context, nodeType *int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if *nodeType == 2 {
				localdata.Lock.Lock()
				validatorNamesCopy := make([]string, len(localdata.ValidatorNames))
				copy(validatorNamesCopy, localdata.ValidatorNames)
				localdata.Lock.Unlock()

				for _, name := range validatorNamesCopy {
					if localdata.GetNodeName() != name {
						salt, err := proofcrypto.CreateRandomHash()
						if err != nil {
							logrus.Errorf("Error creating random hash for ping to %s: %v", name, err)
							continue
						}
						messaging.SendPing(salt, name, nil)
					}
				}
				time.Sleep(120 * time.Second)
			}
		}
	}
}

// RunValidationChallenges - Lightweight validator that challenges storage nodes
// Storage nodes determine their own content based on blockchain/external systems
func RunValidationChallenges(ctx context.Context) {
	logrus.Info("Challenge coordinator started - monitoring storage nodes")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			localdata.Lock.Lock()
			peerNames := localdata.PeerNames
			peerCount := len(peerNames)
			localdata.Lock.Unlock()

			logrus.Debugf("Challenge cycle: found %d storage nodes to test", peerCount)

			if peerCount == 0 {
				logrus.Debug("No storage nodes available for challenges, waiting...")
				time.Sleep(30 * time.Second)
				continue
			}

			// Challenge each storage node with random test
			for _, peer := range peerNames {
				logrus.Debugf("Sending random challenge to storage node: %s", peer)
				go SendRandomChallenge(peer)
				time.Sleep(5 * time.Second) // Stagger challenges
			}

			// Wait before next challenge cycle
			logrus.Debug("Challenge cycle complete, waiting 60 seconds...")
			time.Sleep(60 * time.Second)
		}
	}
}

// SendRandomChallenge - Send a random validation challenge to a storage node
// The storage node decides what content it has and responds accordingly
func SendRandomChallenge(peer string) error {
	logrus.Debugf("Sending random challenge to storage node: %s", peer)

	// Create random salt for this challenge
	salt, err := proofcrypto.CreateRandomHash()
	if err != nil {
		logrus.Errorf("Failed to create random salt for challenge to %s: %v", peer, err)
		return err
	}

	// Create a challenge request - storage node will pick appropriate content
	challengeData := map[string]string{
		"type": "RandomChallenge",
		"hash": salt,
		"user": localdata.GetNodeName(),
	}

	challengeJson, err := json.Marshal(challengeData)
	if err != nil {
		logrus.Errorf("Failed to create challenge JSON for %s: %v", peer, err)
		return err
	}

	logrus.Debugf("Sending random challenge to %s with salt %s", peer, salt)

	// Save challenge start time
	localdata.SaveTime("random-challenge", salt)
	localdata.Lock.Lock()
	localdata.SetStatus(salt, "random-challenge", "Pending", peer, time.Now(), 0)
	localdata.Lock.Unlock()

	// Send challenge via PubSub
	err = pubsub.Publish(string(challengeJson), peer)
	if err != nil {
		logrus.Errorf("Failed to send challenge to %s: %v", peer, err)
		return err
	}

	logrus.Debugf("Random challenge sent to %s, waiting for response...", peer)

	// Wait for response with timeout
	timeout := 15 * time.Second
	startTime := time.Now()

	for {
		if time.Since(startTime) > timeout {
			logrus.Warnf("Challenge to %s timed out after %v", peer, timeout)
			return fmt.Errorf("challenge to %s timed out", peer)
		}

		// Check if we received a response
		status := localdata.GetStatus(salt)
		if status.Status != "" && status.Status != "Pending" {
			logrus.Debugf("Received challenge response from %s: %s", peer, status.Status)
			if status.Status == "Valid" {
				// Successful challenge
				localdata.Lock.Lock()
				localdata.PeerProofs[peer]++
				logrus.Debugf("Challenge successful for %s, total proofs: %d", peer, localdata.PeerProofs[peer])
				localdata.Lock.Unlock()
				return nil
			} else {
				logrus.Debugf("Challenge failed for %s: %s", peer, status.Status)
				return fmt.Errorf("challenge failed: %s", status.Status)
			}
		}

		time.Sleep(100 * time.Millisecond)
	}
}
