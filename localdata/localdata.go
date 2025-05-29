package localdata

import (
	"encoding/json"
	"fmt"
	"proofofaccess/database"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

var Time = time.Now()

const Layout = "2006-01-02 15:04:05.999999 -0700 MST m=+0.000000000"

type Message struct {
	Type   string `json:"type"`
	Hash   string `json:"hash"`
	CID    string `json:"CID"`
	Status string `json:"status"`
}

var Synced = false
var NodeName = ""
var PeerNames = []string{}
var ValidatorNames = []string{}
var ValidatorAddress = map[string]string{}
var PeerStats = map[string][]string{}
var PeerSize = map[string]int{}
var Validators = map[string]bool{}
var ValidatorsStatus = map[string]string{}
var NodesStatus = map[string]string{}
var NodeType int
var PinFileCids = map[string][]string{}
var SavedRefs = map[string][]string{}
var NodeCount = 0
var SyncedPercentage = float32(0)
var WsPeers = map[string]string{}
var UseWS = false
var WsClients = make(map[string]*websocket.Conn)
var WsValidators = make(map[string]*websocket.Conn)
var PingTime = make(map[string]time.Time)
var WsPort = "8000"
var PeerSyncSeed = map[string]int{}
var CIDRefStatus = map[string]bool{}
var CIDRefPercentage = map[string]int{}
var ThreeSpeakVideos = []string{}
var Lock sync.Mutex
var PeerProofs = map[string]int{}
var PeerLastActive = map[string]time.Time{}
var HiveRewarded = map[string]float64{}
var PiningVideos = false
var HoneycombContractCIDs = []string{}
var CidSize = map[string]int{}
var WsWriteMutexes = make(map[string]*sync.Mutex)

type NetworkRecord struct {
	Peers          int    `json:"Peers"`
	NetworkStorage int    `json:"NetworkStorage"`
	Date           string `json:"date"`
}

// SaveTime
// Saves the time to the database using CID + salt as key
func SaveTime(cid string, salt string) {
	Time = time.Now()
	timeStr := Time.Format(Layout)
	key := cid + salt + "time"
	database.Update([]byte(key), []byte(timeStr))
}

// GetTime
// Gets the time from the database using CID + hash (salt) as key
func GetTime(cid string, hash string) time.Time {
	key := cid + hash + "time"
	data := database.Read([]byte(key))
	if data == nil {
		logrus.Warnf("No time found for key %s", key)
		return time.Time{}
	}
	parsedTime, err := time.Parse(Layout, string(data))
	if err != nil {
		logrus.Warnf("Error parsing time layout for key %s (data: %s): %v", key, string(data), err)
		return time.Time{}
	}
	return parsedTime
}

// SetElapsed
// Sets the elapsed time to the database using CID + hash as key
func SetElapsed(cid string, hash string, elapsed time.Duration) {
	key := cid + hash + "elapsed"
	database.Update([]byte(key), []byte(elapsed.String()))
}

// GetElapsed
// Gets the elapsed time from the database using CID + hash as key
func GetElapsed(cid string, hash string) time.Duration {
	key := cid + hash + "elapsed"
	data := database.Read([]byte(key))
	if data == nil {
		logrus.Warnf("No elapsed time found for key %s", key)
		return 0
	}
	parsedDuration, err := time.ParseDuration(string(data))
	if err != nil {
		logrus.Warnf("Error parsing duration for key %s (data: %s): %v", key, string(data), err)
		return 0
	}
	return parsedDuration
}

// GetStatus
// Gets the status from the database (keyed by "Stats"+seed)
func GetStatus(seed string) Message {
	data := database.Read([]byte("Stats" + seed))
	var message Message
	if data == nil {
		logrus.Debugf("No status record found for seed %s", seed)
		return message
	}
	err := json.Unmarshal([]byte(string(data)), &message)
	if err != nil {
		logrus.Errorf("Error decoding Stats JSON for seed %s (data: %s): %v", seed, string(data), err)
	}
	return message
}

// SetStatus
// Sets the final consensus status to the database (keyed by "Stats"+seed).
// Accepts startTime and elapsed duration but might ignore elapsed for consensus records.
func SetStatus(seed string, cid string, status string, name string, startTime time.Time, elapsed time.Duration) {
	timeString := ""
	if !startTime.IsZero() {
		timeString = startTime.Format(time.RFC3339)
	}
	elapsedString := elapsed.String()
	jsonString := fmt.Sprintf(`{"type": "ProofOfAccessConsensus", "CID":"%s", "seed":"%s", "status":"%s", "name":"%s", "time":"%s", "elapsed":"%s"}`,
		cid, seed, status, name, timeString, elapsedString)
	jsonString = strings.TrimSpace(jsonString)
	dbKey := "Stats" + seed
	database.Update([]byte(dbKey), []byte(jsonString))
	logrus.Debugf("SetStatus updated DB key %s with status %s", dbKey, status)
}

// SetNodeName
// Sets the node name in localdata
func SetNodeName(name string) {
	NodeName = name
}

func GetNodeName() string {
	return NodeName
}
func RemoveDuplicates(peerNames []string) []string {
	encountered := map[string]bool{}
	result := []string{}

	for v := range peerNames {
		if encountered[peerNames[v]] == true {
			// Do not add duplicate.
		} else {
			// Record this element as encountered.
			encountered[peerNames[v]] = true
			// Append to result slice.
			result = append(result, peerNames[v])
		}
	}

	return result
}

func RecordNetwork() {
	// Get the current time
	currentTime := time.Now().Format(time.RFC3339)
	NetworkStorage := 0
	Lock.Lock()
	peerNamesCopy := make([]string, len(PeerNames))
	copy(peerNamesCopy, PeerNames)
	for _, peerName := range peerNamesCopy {
		NetworkStorage = NetworkStorage + PeerSize[peerName]
	}
	peers := len(peerNamesCopy)
	Lock.Unlock()

	NetworkStorage = NetworkStorage / 1024 / 1024 / 1024

	record := NetworkRecord{
		Peers:          peers,
		NetworkStorage: NetworkStorage,
		Date:           currentTime,
	}

	jsonRecord, err := json.Marshal(record)
	if err != nil {
		logrus.Errorf("Error encoding NetworkRecord JSON: %v", err)
		return
	}

	database.Save([]byte("Network"+currentTime), jsonRecord)
}
