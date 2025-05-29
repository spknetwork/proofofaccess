package Rewards

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"sync"
	"time"

	"proofofaccess/proofcrypto"
	"proofofaccess/pubsub"
	"proofofaccess/validation"

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

func RunProofs(cids []string) error {
	log.Debugf("RunProofs starting with %d CIDs", len(cids))

	for {
		for i, cid := range cids {
			localdata.Lock.Lock()
			cidStatus := localdata.CIDRefStatus[cid]
			localdata.Lock.Unlock()

			log.Debugf("Processing CID %d/%d: %s (status: %t)", i+1, len(cids), cid, cidStatus)

			if cidStatus == true {
				localdata.Lock.Lock()
				peerNames := localdata.PeerNames
				localdata.Lock.Unlock()

				log.Debugf("CID %s is ready, checking %d peers", cid, len(peerNames))

				for _, peer := range peerNames {
					isPinnedInDB := ipfs.IsPinnedInDB(cid)
					log.Debugf("Checking peer %s for CID %s (pinned in DB: %t)", peer, cid, isPinnedInDB)

					if isPinnedInDB == true {
						// REMOVED: Redundant check of storage node self-reported CID lists
						// Instead, directly challenge storage nodes based on blockchain CID assignments
						// The blockchain/cryptocurrency layer already defines which CIDs each node should store
						log.Debugf("Running proof for peer: %s and CID: %s", peer, cid)
						go RunProof(peer, cid)
						time.Sleep(8 * time.Second)
					} else {
						log.Debugf("Skipping CID %s - not pinned in local database", cid)
					}
				}
			} else {
				log.Debugf("Skipping CID %s - not ready (status: %t)", cid, cidStatus)
			}
		}

		// Add a delay between full cycles
		log.Debug("Completed one full proof cycle, waiting before next cycle...")
		time.Sleep(60 * time.Second)
	}
}

func RunProof(peer string, cid string) error {
	log.Debugf("Initiating proof request for peer: %s and CID: %s", peer, cid)

	// Create a random salt for this proof request
	salt, err := proofcrypto.CreateRandomHash()
	if err != nil {
		log.Errorf("Failed to create random salt for proof request to %s: %v", peer, err)
		return err
	}

	// Create the proof request JSON
	proofJson, err := validation.ProofRequestJson(salt, cid)
	if err != nil {
		log.Errorf("Failed to create proof request JSON for peer %s: %v", peer, err)
		return err
	}

	// Send the proof request to the storage node
	log.Debugf("Sending proof request to storage node %s with salt %s and CID %s", peer, salt, cid)

	// Save the time when we send the request
	localdata.SaveTime(cid, salt)

	// Set initial status to Pending
	localdata.Lock.Lock()
	localdata.SetStatus(salt, cid, "Pending", peer, time.Now(), 0)
	localdata.Lock.Unlock()

	// Send the request via PubSub (storage nodes listen on their own name)
	err = pubsub.Publish(string(proofJson), peer)
	if err != nil {
		log.Errorf("Failed to send proof request to storage node %s: %v", peer, err)
		return err
	}

	log.Debugf("Proof request sent to storage node %s, waiting for response...", peer)

	// Wait for response with timeout
	timeout := 15 * time.Second
	startTime := time.Now()

	for {
		if time.Since(startTime) > timeout {
			log.Warnf("Proof request to %s timed out after %v", peer, timeout)
			return fmt.Errorf("proof request to %s timed out", peer)
		}

		// Check if we received a response
		status := localdata.GetStatus(salt)
		if status.Status != "" && status.Status != "Pending" {
			log.Debugf("Received proof response from %s: %s", peer, status.Status)
			if status.Status == "Valid" {
				// If proof is successful, add to localdata.PeerProofs
				localdata.Lock.Lock()
				localdata.PeerProofs[peer]++
				log.Debugf("Proof successful for peer %s, total proofs: %d", peer, localdata.PeerProofs[peer])
				localdata.Lock.Unlock()
				return nil
			} else {
				log.Debugf("Proof failed for peer %s: %s", peer, status.Status)
				return fmt.Errorf("proof failed: %s", status.Status)
			}
		}

		time.Sleep(100 * time.Millisecond) // Check every 100ms
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

			RunProofs(cids)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
