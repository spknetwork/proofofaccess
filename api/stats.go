package api

import (
	"encoding/json"
	"fmt"
	"math"
	"proofofaccess/localdata"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

func stats(c *websocket.Conn, key string) {
	NetworkStorage := 0
	localdata.Lock.Lock()
	if localdata.NodeType == 2 {
		NetworkStorage = localdata.PeerSize[localdata.NodeName]
	} else {
		for _, peerName := range localdata.PeerNames {
			NetworkStorage = NetworkStorage + localdata.PeerSize[peerName]
		}
	}
	peerSizes := make(map[string]string)
	peerSynced := make(map[string]string)
	for _, peerName := range localdata.PeerNames {
		peerSizes[peerName] = fmt.Sprintf("%d", localdata.PeerSize[peerName]/1024/1024/1024)
		peerSynced[peerName] = fmt.Sprintf("%v", localdata.NodesStatus[peerName])
	}
	synced := localdata.Synced
	peersCount := len(localdata.PeerNames)
	validatorCount := len(localdata.ValidatorNames)
	nodeName := localdata.NodeName
	nodeType := localdata.NodeType
	peerNames := make([]string, len(localdata.PeerNames))
	copy(peerNames, localdata.PeerNames)
	validatorNames := make([]string, len(localdata.ValidatorNames))
	copy(validatorNames, localdata.ValidatorNames)
	syncedPercentage := localdata.SyncedPercentage
	peerLastActive := make(map[string]time.Time)
	for k, v := range localdata.PeerLastActive {
		peerLastActive[k] = v
	}
	peerProofs := make(map[string]int)
	for k, v := range localdata.PeerProofs {
		peerProofs[k] = v
	}
	hiveRewarded := make(map[string]int)
	for k, v := range localdata.HiveRewarded {
		hiveRewarded[k] = int(v)
	}

	localdata.Lock.Unlock()

	NetworkStorage = NetworkStorage / 1024 / 1024 / 1024
	NodeTypeStr := ""
	if nodeType == 1 {
		NodeTypeStr = "Validator"
	} else {
		NodeTypeStr = "Storage"
	}
	data := map[string]interface{}{
		"Status": map[string]string{
			"Sync":             fmt.Sprintf("%v", synced),
			"PeersCount":       fmt.Sprintf("%d", peersCount),
			"ValidatorCount":   fmt.Sprintf("%d", validatorCount),
			"Node":             fmt.Sprintf(nodeName),
			"Type":             fmt.Sprintf(NodeTypeStr),
			"Peers":            strings.Join(peerNames, ","),
			"Validators":       strings.Join(validatorNames, ","),
			"NetworkStorage":   fmt.Sprintf("%d GB", NetworkStorage),
			"SyncedPercentage": fmt.Sprintf("%f", math.Round(float64(syncedPercentage))),
		},
		"PeerSizes":       peerSizes,
		"PeerLastActive":  peerLastActive,
		"PeerProofs":      peerProofs,
		"PeerSynced":      peerSynced,
		"PeerHiveRewards": hiveRewarded,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		logrus.Errorf("Error marshaling stats JSON: %v", err)
		return
	}

	err = c.WriteMessage(websocket.TextMessage, jsonData)
	if err != nil {
		logrus.Errorf("Error writing stats message to WebSocket: %v", err)
	}

}
