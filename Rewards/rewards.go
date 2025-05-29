package Rewards

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"proofofaccess/localdata"
	"sync"
	"time"

	"proofofaccess/proofcrypto"
	"proofofaccess/pubsub"

	log "github.com/sirupsen/logrus"
)

type APIResponse struct {
	Recommended []struct {
		VideoV2 string `json:"video_v2"`
	} `json:"recommended"`
}
type ProofMessage struct {
	Status  string `json:"Status"`
	Message string `json:"Message"`
	Elapsed string `json:"Elapsed"`
}
type HiveTransfer struct {
	Username string `json:"username"`
	Amount   string `json:"amount"`
}

// Create a WaitGroup to wait for the function to finish
var wg sync.WaitGroup

// RunValidationChallenges - Lightweight validator that challenges storage nodes
// Storage nodes determine their own content based on blockchain/external systems
func RunValidationChallenges(ctx context.Context) {
	log.Info("Starting lightweight validation challenge coordinator")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			localdata.Lock.Lock()
			peerNames := localdata.PeerNames
			peerCount := len(peerNames)
			localdata.Lock.Unlock()

			log.Debugf("Challenge cycle: found %d storage nodes to test", peerCount)

			if peerCount == 0 {
				log.Debug("No storage nodes available for challenges, waiting...")
				time.Sleep(30 * time.Second)
				continue
			}

			// Challenge each storage node with random test
			for _, peer := range peerNames {
				log.Debugf("Sending random challenge to storage node: %s", peer)
				go SendRandomChallenge(peer)
				time.Sleep(5 * time.Second) // Stagger challenges
			}

			// Wait before next challenge cycle
			log.Debug("Challenge cycle complete, waiting 60 seconds...")
			time.Sleep(60 * time.Second)
		}
	}
}

// SendRandomChallenge - Send a random validation challenge to a storage node
// The storage node decides what content it has and responds accordingly
func SendRandomChallenge(peer string) error {
	log.Debugf("Sending random challenge to storage node: %s", peer)

	// Create random salt for this challenge
	salt, err := proofcrypto.CreateRandomHash()
	if err != nil {
		log.Errorf("Failed to create random salt for challenge to %s: %v", peer, err)
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
		log.Errorf("Failed to create challenge JSON for %s: %v", peer, err)
		return err
	}

	log.Debugf("Sending random challenge to %s with salt %s", peer, salt)

	// Save challenge start time
	localdata.SaveTime("random-challenge", salt)
	localdata.Lock.Lock()
	localdata.SetStatus(salt, "random-challenge", "Pending", peer, time.Now(), 0)
	localdata.Lock.Unlock()

	// Send challenge via PubSub
	err = pubsub.Publish(string(challengeJson), peer)
	if err != nil {
		log.Errorf("Failed to send challenge to %s: %v", peer, err)
		return err
	}

	log.Debugf("Random challenge sent to %s, waiting for response...", peer)

	// Wait for response with timeout
	timeout := 15 * time.Second
	startTime := time.Now()

	for {
		if time.Since(startTime) > timeout {
			log.Warnf("Challenge to %s timed out after %v", peer, timeout)
			return fmt.Errorf("challenge to %s timed out", peer)
		}

		// Check if we received a response
		status := localdata.GetStatus(salt)
		if status.Status != "" && status.Status != "Pending" {
			log.Debugf("Received challenge response from %s: %s", peer, status.Status)
			if status.Status == "Valid" {
				// Successful challenge
				localdata.Lock.Lock()
				localdata.PeerProofs[peer]++
				log.Debugf("Challenge successful for %s, total proofs: %d", peer, localdata.PeerProofs[peer])
				localdata.Lock.Unlock()
				return nil
			} else {
				log.Debugf("Challenge failed for %s: %s", peer, status.Status)
				return fmt.Errorf("challenge failed: %s", status.Status)
			}
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func RewardPeers() {
	for {
		//fmt.Println("Rewarding peers")
		for _, peer := range localdata.PeerNames {
			//fmt.Println("Checking proofs for peer: " + peer)
			localdata.Lock.Lock()
			proofs := localdata.PeerProofs[peer]
			//fmt.Println("Proofs: ", proofs)
			if proofs >= 10 {
				//fmt.Println("Rewarding peer: " + peer)
				localdata.PeerProofs[peer] = localdata.PeerProofs[peer] - 10 // Update the map while the lock is held

				// Creating the request body
				transfer := HiveTransfer{
					Username: peer,
					Amount:   "0.050",
				}

				reqBody, err := json.Marshal(transfer)
				if err != nil {
					log.Errorf("Error marshaling request body: %v", err)
					continue
				}

				// Making the POST request
				resp, err := http.Post("http://localhost:3000/send-hive", "application/json", bytes.NewBuffer(reqBody))
				if err != nil {
					log.Errorf("Error sending hive: %v", err)
					continue
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					log.Warnf("Non-OK HTTP status: %d", resp.StatusCode)
					continue
				}
				localdata.HiveRewarded[peer] = localdata.HiveRewarded[peer] + 0.050
				log.Debugf("Rewarded hive to peer: %s", peer)
			}
			localdata.Lock.Unlock()
		}
		time.Sleep(10 * time.Second)
	}
}

type CIDSize struct {
	CID  string
	Size int64
}

func Update(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			localdata.RecordNetwork()
			ThreeSpeak()
			time.Sleep(600 * time.Second)
		}
	}
}

func RunRewardProofs(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Debug("Running reward proofs cycle...")
			localdata.Lock.Lock()
			cids := localdata.ThreeSpeakVideos
			peerCount := len(localdata.PeerNames)
			localdata.Lock.Unlock()

			log.Debugf("Starting proofs for %d CIDs with %d peers", len(cids), peerCount)
			if len(cids) > 0 {
				log.Debugf("Sample CIDs: %v", cids[:min(3, len(cids))])
			}
			if peerCount == 0 {
				log.Debug("No peers available for proof validation, waiting...")
				time.Sleep(30 * time.Second)
				continue
			}

			RunValidationChallenges(ctx)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
