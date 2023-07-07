package Rewards

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io/ioutil"
	"net/http"
	"net/url"
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

func ThreeSpeak() {
	hashes := []string{}
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
			hashes = append(hashes, hash)
		}
	}

	localdata.ThreeSpeakVideos = hashes
	return
}

func RunProofs() error {
	for _, peer := range localdata.PeerNames {
		fmt.Println("Running proofs for peer: " + peer)
		for _, cid := range localdata.ThreeSpeakVideos {
			localdata.Lock.Lock()
			for _, peerHash := range localdata.PeerCids[peer] {
				localdata.Lock.Unlock()
				if peerHash == cid {
					proof, err := runProofAPI(peer, cid)
					if err != nil {
						return fmt.Errorf("failed to run proof for peer %s and CID %s: %w", peer, cid, err)
					}

					// If proof is successful, add to localdata.PeerProofs
					if proof.Success {
						fmt.Println("Proof successful")
						fmt.Println("Peer: " + peer)
						fmt.Println("CID: " + cid)

					}
				}
			}

		}
	}
	//wait 5 seconds between peers
	time.Sleep(1 * time.Second)
	return nil
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
