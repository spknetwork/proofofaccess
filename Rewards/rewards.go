package Rewards

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io/ioutil"
	"net/http"
	"net/url"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"strings"
	"time"
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

func ThreeSpeak() {
	hashSet := make(map[string]struct{})
	for skip := 0; skip <= 2000; skip += 40 {
		resp, err := http.Get(fmt.Sprintf("https://3speak.tv/api/new/more?skip=%d", skip))
		if err != nil {
			fmt.Println(err)
			continue
		}

		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Println(err)
			continue
		}

		var apiResponse APIResponse
		err = json.Unmarshal(body, &apiResponse)
		if err != nil {
			fmt.Println(err)
			continue
		}

		for _, rec := range apiResponse.Recommended {
			hash := strings.TrimPrefix(strings.TrimSuffix(rec.VideoV2, "/manifest.m3u8"), "ipfs://")
			if hash != `` {
				hashSet[hash] = struct{}{}
			}
		}
	}

	localdata.ThreeSpeakVideos = make([]string, 0, len(hashSet))
	for hash := range hashSet {
		localdata.ThreeSpeakVideos = append(localdata.ThreeSpeakVideos, hash)
	}
}

func RunProofs() error {
	for {
		//fmt.Println("Running proofs")
		//fmt.Println("length of localdata.PeerNames: " + strconv.Itoa(len(localdata.PeerNames)))
		//fmt.Println("Length of ThreeSpeakVideos: " + strconv.Itoa(len(localdata.ThreeSpeakVideos)))
		for _, cid := range localdata.ThreeSpeakVideos {
			fmt.Println("Running proofs for CID: " + cid)
			for _, peer := range localdata.PeerNames {
				//fmt.Println("Running proofs for peer: " + peer)
				isPinnedInDB := ipfs.IsPinnedInDB(cid)
				if isPinnedInDB == true {
					//fmt.Println("Running proofs for peer: " + peer)
					//fmt.Println("Length of PeerCids: " + strconv.Itoa(len(localdata.PeerCids[peer])))
					localdata.Lock.Lock()
					peers := localdata.PeerCids[peer]
					localdata.Lock.Unlock()
					for _, peerHash := range peers {
						if peerHash == cid {
							fmt.Println("Running proof for peer: " + peer + " and CID: " + cid)
							go RunProof(peer, cid)
							time.Sleep(4 * time.Second)
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
					fmt.Println("Error marshaling request body:", err)
					continue
				}

				// Making the POST request
				resp, err := http.Post("http://localhost:3000/send-hive", "application/json", bytes.NewBuffer(reqBody))
				if err != nil {
					fmt.Println("Error sending hive:", err)
					continue
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					fmt.Println("Non-OK HTTP status:", resp.StatusCode)
					continue
				}
				localdata.HiveRewarded[peer] = localdata.HiveRewarded[peer] + 0.050
				fmt.Println("Rewarded hive to peer: " + peer)
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
			fmt.Println("Valid")
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

func PinVideos(gb int, ctx context.Context) error {
	fmt.Println("Pinning videos")
	sh := ipfs.Shell
	const GB = 1024 * 1024 * 1024
	limit := int64(gb * GB)
	totalPinned := int64(0)

	// Convert ThreeSpeakVideos to a map for quicker lookups
	videoMap := make(map[string]bool)
	for _, cid := range localdata.ThreeSpeakVideos {
		videoMap[cid] = true
	}

	// Check all the currently pinned CIDs
	fmt.Println("Checking currently pinned CIDs")
	allPinsData, _ := sh.Pins()

	// Map the allPins to only CIDs
	allPins := make(map[string]bool)
	for cid := range allPinsData {
		allPins[cid] = true
	}

	for cid, pinInfo := range allPinsData {
		// Filter only the direct pins
		if pinInfo.Type == "recursive" {
			if videoMap[cid] {
				fmt.Println("CID is in the video list", cid)
				// If the CID is in the video list, get its size and add to totalPinned
				stat, err := sh.ObjectStat(cid)
				if err != nil {
					fmt.Printf("Error getting stats for CID %s: %s\n", cid, err)
					continue
				}
				size := int64(stat.CumulativeSize)
				totalPinned += size
				fmt.Println("Total pinned: ", totalPinned)
			} else {
				// If the CID is not in the video list, unpin it
				fmt.Println("Unpinning CID: ", cid)
				if err := sh.Unpin(cid); err != nil {
					fmt.Printf("Error unpinning CID %s: %s\n", cid, err)
					continue
				}
				fmt.Println("Unpinned CID: ", cid)
			}
		}
	}
	fmt.Println("Total pinned: ", totalPinned)

	// Pin videos from ThreeSpeak until the limit is reached
	for _, cid := range localdata.ThreeSpeakVideos {
		if totalPinned >= limit {
			fmt.Println("Total pinned is greater than limit")
			break
		}
		fmt.Println("Getting stats for CID: ", cid)
		// If CID isn't already in allPins, then it's not pinned
		if _, pinned := allPins[cid]; !pinned {
			stat, err := sh.ObjectStat(cid)
			if err != nil {
				fmt.Printf("Error getting stats for CID %s: %s\n", cid, err)
				continue
			}

			size := int64(stat.CumulativeSize)
			fmt.Println("Size: ", size)
			fmt.Println("Total pinned: ", totalPinned)
			fmt.Println("Limit: ", limit)
			if totalPinned+size-1000000 <= limit {
				fmt.Println("Pinning CID: ", cid)
				if err := sh.Pin(cid); err != nil {
					fmt.Printf("Failed to pin CID %s: %s\n", cid, err)
					continue
				}
				totalPinned += size
				// Once pinned, add it to the allPins
				allPins[cid] = true
				fmt.Println("Pinned CID: ", cid)
			}
		} else {
			fmt.Println("CID is already pinned")
		}
	}

	return nil
}
