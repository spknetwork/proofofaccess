package messaging

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/proofcrypto"
	"proofofaccess/pubsub"
	"strconv"
	"strings"
)

func RequestCIDS(req Request) {
	data := map[string]string{
		"type": "RequestCIDS",
		"user": localdata.GetNodeName(),
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error encoding JSON:", err)
		return
	}
	if localdata.WsPeers[req.User] == req.User && localdata.NodeType == 1 {
		wsMutex.Lock()
		ws := localdata.WsClients[req.User]
		ws.WriteMessage(websocket.TextMessage, jsonData)
		wsMutex.Unlock()
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		wsMutex.Lock()
		localdata.WsValidators[req.User].WriteMessage(websocket.TextMessage, jsonData)
		wsMutex.Unlock()
	} else {
		pubsub.Publish(string(jsonData), req.User)
	}

}
func SendCIDS(name string) {
	allPins, _ := ipfs.Shell.Pins()
	fmt.Println("Fetched pins")
	NewPins := make([]string, 0)
	for key, pinInfo := range allPins {
		if pinInfo.Type == "recursive" {
			NewPins = append(NewPins, key)
		}
	}
	fmt.Println("Sending CIDS")
	pinsJson, err := json.Marshal(NewPins)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Split the pinsJson into smaller chunks
	chunks := splitIntoChunks(string(pinsJson), 3000) // 1000 is the chunk size, adjust as needed
	seed, _ := proofcrypto.CreateRandomHash()
	for i, chunk := range chunks {
		data := map[string]string{
			"type":       "SendCIDS",
			"user":       localdata.GetNodeName(),
			"seed":       seed,
			"pins":       chunk,
			"part":       strconv.Itoa(i + 1),
			"totalParts": strconv.Itoa(len(chunks)),
		}

		jsonData, err := json.Marshal(data)
		if err != nil {
			fmt.Println("Error encoding JSON:", err)
			return
		}

		if localdata.UseWS == true && localdata.NodeType == 2 {
			localdata.Lock.Lock()
			ws := localdata.WsValidators[name]
			ws.WriteJSON(data)
			localdata.Lock.Unlock()
		} else {
			pubsub.Publish(string(jsonData), name)
		}
	}
}

// splitIntoChunks splits a string into chunks of the specified size
func splitIntoChunks(s string, chunkSize int) []string {
	var chunks []string
	runes := []rune(s)

	for i := 0; i < len(runes); {
		end := i + chunkSize

		if end > len(runes) {
			end = len(runes)
		}

		// Check if the end index is in the middle of a CID
		if end < len(runes) && runes[end] != ',' {
			for end < len(runes) && runes[end] != ',' {
				end++
			}
		}

		// Remove leading and trailing commas, and enclose chunk in brackets
		chunk := string(runes[i:end])
		chunk = strings.Trim(chunk, ",")
		chunk = strings.Trim(chunk, "[")
		chunk = strings.Trim(chunk, "]")
		chunk = "[" + chunk + "]"

		chunks = append(chunks, chunk)

		// Move the start index to the next CID
		i = end
		if i < len(runes) && runes[i] == ',' {
			i++
		}
	}

	return chunks
}
