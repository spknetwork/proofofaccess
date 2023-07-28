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
	"strconv"
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
		fmt.Println("Running proofs")
		fmt.Println("length of localdata.PeerNames: " + strconv.Itoa(len(localdata.PeerNames)))
		fmt.Println("Length of ThreeSpeakVideos: " + strconv.Itoa(len(localdata.ThreeSpeakVideos)))
		for _, cid := range localdata.ThreeSpeakVideos {
			for _, peer := range localdata.PeerNames {
				localdata.Lock.Lock()
				nodeStatus := localdata.NodesStatus[peer]
				localdata.Lock.Unlock()
				if nodeStatus == "Synced" {
					fmt.Println("Running proofs for peer: " + peer)
					fmt.Println("Length of PeerCids: " + strconv.Itoa(len(localdata.PeerCids[peer])))
					localdata.Lock.Lock()
					peers := localdata.PeerCids[peer]
					localdata.Lock.Unlock()
					for _, peerHash := range peers {
						if peerHash == cid {
							fmt.Println("Running proof for peer: " + peer + " and CID: " + cid)
							go RunProof(peer, cid)
							time.Sleep(10 * time.Second)
						}
					}
				}
				//wait 10 seconds between peers
				time.Sleep(10 * time.Second)
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
		fmt.Println("Rewarding peers")
		for _, peer := range localdata.PeerNames {
			fmt.Println("Checking proofs for peer: " + peer)
			localdata.Lock.Lock()
			proofs := localdata.PeerProofs[peer]
			fmt.Println("Proofs: ", proofs)
			if proofs >= 10 {
				fmt.Println("Rewarding peer: " + peer)
				localdata.PeerProofs[peer] = localdata.PeerProofs[peer] - 10 // Update the map while the lock is held

				// Creating the request body
				transfer := HiveTransfer{
					Username: peer,
					Amount:   "0.025",
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
	// Connect to the local IPFS node
	sh := ipfs.Shell

	// Calculate total pinned storage
	totalPinned := int64(0)
	fmt.Println("Getting pins")
	pins, err := sh.Pins()
	if err != nil {
		fmt.Println("Error getting pins")
		fmt.Println(err)
	}
	fmt.Println("Got pins")

	// Define the limit for the pinned storage
	const GB = 1024 * 1024 * 1024
	limit := int64(gb * GB)
	fmt.Println("Limit: ", strconv.FormatInt(limit, 10))
	// Generate list of CIDs with size
	cidList := make([]CIDSize, len(localdata.ThreeSpeakVideos))
	fmt.Println("Making CID list")
	fmt.Println("Length of localdata.ThreeSpeakVideos: ")
	fmt.Println(len(localdata.ThreeSpeakVideos))

	for i, cid := range localdata.ThreeSpeakVideos {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if cid == "" {
				fmt.Printf("Empty CID at index %d, skipping\n", i)
				continue
			}
			fmt.Println("CID: " + cid)
			stat, err := sh.ObjectStat(cid)
			fmt.Println("Got object stats ", stat)
			if err != nil {
				fmt.Printf("Failed to get object stats for CID %s: %v, skipping\n", cid, err)
				continue
			}

			cidList[i] = CIDSize{
				CID:  cid,
				Size: int64(stat.CumulativeSize),
			}
			totalPinned += int64(stat.CumulativeSize)
			fmt.Println("Total pinned: " + strconv.FormatInt(totalPinned, 10))
		}
	}

	fmt.Println("Got CID list")

	// Pin new videos until limit is reached
	for _, video := range cidList {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if totalPinned+video.Size > limit {
				fmt.Println("Total pinned storage exceeds limit")
				break
			}
			fmt.Println("Pinning CID: " + video.CID)
			if err := sh.Pin(video.CID); err != nil {
				fmt.Println("failed to pin CID %s: %w", video.CID, err)
			}
			fmt.Println("Pinned CID: " + video.CID)
			totalPinned += video.Size
		}
	}

	// Remove older videos if total pinned storage exceeds limit
	for cid := range pins {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			stat, err := sh.ObjectStat(cid)
			if err != nil {
				fmt.Println("failed to get object stats for CID %s: %w", cid, err)
			}
			if totalPinned <= limit {
				break
			}
			if err := sh.Unpin(cid); err != nil {
				fmt.Println("failed to unpin CID %s: %w", cid, err)
			}
			totalPinned -= int64(stat.CumulativeSize) // Use actual size of the unpinned video
		}
	}

	return nil
}
