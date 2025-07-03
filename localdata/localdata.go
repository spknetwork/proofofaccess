package localdata

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

var Time = time.Now()

const Layout = "2006-01-02 15:04:05.999999 -0700 MST m=+0.000000000"

type Message struct {
	Type      string    `json:"type"`
	Hash      string    `json:"hash"`
	CID       string    `json:"CID"`
	Status    string    `json:"status"`
	Name      string    `json:"name"`
	Time      time.Time `json:"time"`
	Elapsed   string    `json:"elapsed"`
	Timestamp int64     `json:"timestamp"`
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
var Lock sync.Mutex
var PeerProofs = map[string]int{}
var PeerLastActive = map[string]time.Time{}
var PiningVideos = false
var HoneycombContractCIDs = []string{}
var CidSize = map[string]int{}
var WsWriteMutexes = make(map[string]*sync.Mutex)

// In-memory storage for validators (when database is not available)
var inMemoryStorage = make(map[string][]byte)
var inMemoryMutex sync.RWMutex

type NetworkRecord struct {
	Peers          int    `json:"Peers"`
	NetworkStorage int    `json:"NetworkStorage"`
	Date           string `json:"date"`
}


// saveData saves data to in-memory storage
func saveData(key string, value []byte) {
	inMemoryMutex.Lock()
	inMemoryStorage[key] = make([]byte, len(value))
	copy(inMemoryStorage[key], value)
	inMemoryMutex.Unlock()
	logrus.Debugf("Saved to in-memory storage: key=%s", key)
}

// readData reads data from in-memory storage
func readData(key string) []byte {
	inMemoryMutex.RLock()
	value, exists := inMemoryStorage[key]
	inMemoryMutex.RUnlock()
	if !exists {
		logrus.Debugf("Key not found in in-memory storage: %s", key)
		return nil
	}
	// Return a copy to avoid modification issues
	result := make([]byte, len(value))
	copy(result, value)
	logrus.Debugf("Read from in-memory storage: key=%s", key)
	return result
}

// SaveTime
// Saves the time to the database using CID + salt as key
func SaveTime(cid string, salt string) {
	Time = time.Now()
	timeStr := Time.Format(Layout)
	key := cid + salt + "time"
	saveData(key, []byte(timeStr))
}

// GetTime
// Gets the time from the database using CID + hash (salt) as key
func GetTime(cid string, hash string) time.Time {
	key := cid + hash + "time"
	data := readData(key)
	if data == nil {
		logrus.Debugf("No time found for key %s", key)
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
	saveData(key, []byte(elapsed.String()))
}

// GetElapsed
// Gets the elapsed time from the database using CID + hash as key
func GetElapsed(cid string, hash string) time.Duration {
	key := cid + hash + "elapsed"
	data := readData(key)
	if data == nil {
		logrus.Debugf("No elapsed time found for key %s", key)
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
	data := readData("Stats" + seed)
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
	// Add timestamp for TTL
	timestamp := time.Now().Unix()
	jsonString := fmt.Sprintf(`{"type": "ProofOfAccessConsensus", "CID":"%s", "seed":"%s", "status":"%s", "name":"%s", "time":"%s", "elapsed":"%s", "timestamp":%d}`,
		cid, seed, status, name, timeString, elapsedString, timestamp)
	jsonString = strings.TrimSpace(jsonString)
	dbKey := "Stats" + seed
	saveData(dbKey, []byte(jsonString))
	logrus.Debugf("SetStatus updated storage key %s with status %s", dbKey, status)
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

	// Save network records to in-memory storage
	saveData("Network"+currentTime, jsonRecord)
	logrus.Debugf("Saved network record for time: %s", currentTime)
}

// CleanupOldValidations removes validation results older than the specified duration
func CleanupOldValidations(maxAge time.Duration) {
	now := time.Now()
	cutoff := now.Add(-maxAge).Unix()
	
	inMemoryMutex.Lock()
	defer inMemoryMutex.Unlock()
	
	keysToDelete := []string{}
	
	for key, data := range inMemoryStorage {
		if strings.HasPrefix(key, "Stats") {
			// Try to parse the validation result
			var result map[string]interface{}
			if err := json.Unmarshal(data, &result); err == nil {
				if timestamp, ok := result["timestamp"].(float64); ok {
					if int64(timestamp) < cutoff {
						keysToDelete = append(keysToDelete, key)
					}
				}
			}
		}
	}
	
	for _, key := range keysToDelete {
		delete(inMemoryStorage, key)
		logrus.Debugf("Cleaned up old validation: %s", key)
	}
	
	if len(keysToDelete) > 0 {
		logrus.Infof("Cleaned up %d old validation results", len(keysToDelete))
	}
}

// GetStats returns validation statistics for API handlers
func GetStats(user string) []Message {
	inMemoryMutex.RLock()
	defer inMemoryMutex.RUnlock()
	
	var messages []Message
	for key, data := range inMemoryStorage {
		if strings.HasPrefix(key, "Stats") {
			// Parse the raw JSON to get all fields
			var rawMsg map[string]interface{}
			if err := json.Unmarshal(data, &rawMsg); err != nil {
				continue
			}
			
			msg := Message{
				Type:   getStringField(rawMsg, "type"),
				CID:    getStringField(rawMsg, "CID"),
				Status: getStringField(rawMsg, "status"),
				Name:   getStringField(rawMsg, "name"),
				Elapsed: getStringField(rawMsg, "elapsed"),
			}
			
			// Parse seed as Hash
			if seed, ok := rawMsg["seed"].(string); ok {
				msg.Hash = seed
			}
			
			// Parse time
			if timeStr, ok := rawMsg["time"].(string); ok && timeStr != "" {
				if parsedTime, err := time.Parse(time.RFC3339, timeStr); err == nil {
					msg.Time = parsedTime
				}
			}
			
			// Parse timestamp
			if timestamp, ok := rawMsg["timestamp"].(float64); ok {
				msg.Timestamp = int64(timestamp)
			}
			
			if user == "" || msg.Name == user {
				messages = append(messages, msg)
			}
		}
	}
	return messages
}

func getStringField(m map[string]interface{}, field string) string {
	if val, ok := m[field].(string); ok {
		return val
	}
	return ""
}

// GetNetwork returns network statistics for API handlers
func GetNetwork() []NetworkRecord {
	inMemoryMutex.RLock()
	defer inMemoryMutex.RUnlock()
	
	var records []NetworkRecord
	for key, data := range inMemoryStorage {
		if strings.HasPrefix(key, "Network") {
			var record NetworkRecord
			if err := json.Unmarshal(data, &record); err == nil {
				records = append(records, record)
			}
		}
	}
	return records
}
