package messaging

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/proofcrypto"
	"proofofaccess/pubsub"
	"strconv"
	"strings"
)

func RequestCIDS(req Request, ws *websocket.Conn) {
	data := map[string]string{
		"type": "RequestCIDS",
		"user": localdata.GetNodeName(),
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		logrus.Errorf("Error encoding RequestCIDS JSON: %v", err)
		return
	}
	if localdata.WsPeers[req.User] == req.User && localdata.NodeType == 1 {
		WsMutex.Lock()
		err = ws.WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			logrus.Errorf("Error writing RequestCIDS to WebSocket for %s: %v", req.User, err)
		}
		WsMutex.Unlock()
	} else if localdata.UseWS == true && localdata.NodeType == 2 {
		WsMutex.Lock()
		err = localdata.WsValidators[req.User].WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			logrus.Errorf("Error writing RequestCIDS to WebSocket validator %s: %v", req.User, err)
		}
		WsMutex.Unlock()
	} else {
		pubsub.Publish(string(jsonData), req.User)
	}

}
func SendCIDS(name string, ws *websocket.Conn) {
	allPins, err := ipfs.Shell.Pins()
	if err != nil {
		logrus.Errorf("Error fetching pins in SendCIDS: %v", err)
		return
	}
	NewPins := make([]string, 0)
	for key, pinInfo := range allPins {
		if pinInfo.Type == "recursive" {
			NewPins = append(NewPins, key)
		}
	}
	pinsJson, err := json.Marshal(NewPins)
	if err != nil {
		logrus.Errorf("Error marshaling pins in SendCIDS: %v", err)
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
			logrus.Errorf("Error encoding SendCIDS JSON chunk: %v", err)
			return
		}

		if localdata.UseWS == true && localdata.NodeType == 2 {
			WsMutex.Lock()
			err = ws.WriteJSON(data)
			if err != nil {
				logrus.Errorf("Error writing SendCIDS JSON to WebSocket for %s: %v", name, err)
			}
			WsMutex.Unlock()
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
