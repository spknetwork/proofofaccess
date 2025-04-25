package Rewards

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"sync"
	"time"

	"github.com/gorilla/websocket"
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
	for {
		//fmt.Println("Running proofs")
		//fmt.Println("length of localdata.PeerNames: " + strconv.Itoa(len(localdata.PeerNames)))
		//fmt.Println("Length of ThreeSpeakVideos: " + strconv.Itoa(len(localdata.ThreeSpeakVideos)))
		for _, cid := range cids {
			localdata.Lock.Lock()
			cidStatus := localdata.CIDRefStatus[cid]
			localdata.Lock.Unlock()
			if cidStatus == true {
				// fmt.Println("Running proofs for CID: " + cid)
				localdata.Lock.Lock()
				peerNames := localdata.PeerNames
				localdata.Lock.Unlock()
				for _, peer := range peerNames {
					//fmt.Println("Running proof for peer: " + peer)
					isPinnedInDB := ipfs.IsPinnedInDB(cid)
					if isPinnedInDB == true {
						//fmt.Println("Running proofs for peer: " + peer)
						//fmt.Println("Length of PeerCids: " + strconv.Itoa(len(localdata.PeerCids[peer])))
						localdata.Lock.Lock()
						peers := localdata.PeerCids[peer]
						localdata.Lock.Unlock()
						for _, peerHash := range peers {
							if peerHash == cid {
								log.Infof("Running proof for peer: %s and CID: %s", peer, cid)
								go RunProof(peer, cid)
								time.Sleep(8 * time.Second)
							}
						}
					}
				}
			}
		}
	}
}

func RunProof(peer string, cid string) error {
	proof, err := runProofAPI(peer, cid)
	if err != nil {
		return fmt.Errorf("failed to run proof for peer %s and CID %s: %w", peer, cid, err)
	}
	// If proof is successful, add to localdata.PeerProofs
	if proof.Success {
		//fmt.Println("Proof successful")
		//fmt.Println("Peer: " + peer)
		//fmt.Println("CID: " + cid)
		localdata.Lock.Lock()
		peerProofs := localdata.PeerProofs[peer]
		peerProofs = peerProofs + 1
		//fmt.Println("Proofs: ", peerProofs)
		localdata.PeerProofs[peer] = peerProofs // Update the map while the lock is held
		localdata.Lock.Unlock()
	}
	return nil
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
				log.Infof("Rewarded hive to peer: %s", peer)
			}
			localdata.Lock.Unlock()
		}
		time.Sleep(10 * time.Second)
	}
}

type Proof struct {
	Success bool
}

func runProofAPI(peer string, cid string) (*Proof, error) {
	u := url.URL{Scheme: "ws", Host: "localhost:8000", Path: "/validate"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the WebSocket server: %w", err)
	}
	defer c.Close()

	clientInfo := map[string]string{
		"CID":  cid,
		"Name": peer,
	}

	err = c.WriteJSON(clientInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to send client information to the server: %w", err)
	}

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("failed to read message from the server: %w", err)
		}

		var proofMessage ProofMessage
		err = json.Unmarshal(message, &proofMessage)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal server message: %w", err)
		}

		// Stop processing when receiving a "Valid" message, but keep the connection open to receive other messages.
		if proofMessage.Status == "Valid" {
			log.Info("Valid")
			break
		}

		// Add a delay before sending the next request, if needed.
		time.Sleep(time.Second)
	}

	return &Proof{Success: true}, nil
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
			log.Info("Running proofs...")
			localdata.Lock.Lock()
			cids := localdata.ThreeSpeakVideos
			localdata.Lock.Unlock()
			RunProofs(cids)
		}
	}
}
