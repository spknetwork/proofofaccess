package localdata

import (
	"encoding/json"
	"fmt"
	"proofofaccess/database"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
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
var PeerCids = map[string][]string{}
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
// Saves the time to the database
func SaveTime(salt string) {
	Time = time.Now()
	timeStr := Time.Format(Layout)
	//fmt.Println("Time: ", timeStr)
	//fmt.Println("Salt: ", salt)
	database.Update([]byte(salt+"time"), []byte(timeStr))
}

// GetTime
// Gets the time from the database
func GetTime(hash string) time.Time {
	data := database.Read([]byte(hash + "time"))
	//fmt.Println("GetTime", string(data))
	//fmt.Println("salt", hash)
	parsedTime, _ := time.Parse(Layout, string(data))
	return parsedTime
}

// SetElapsed
// Sets the elapsed time to the database
func SetElapsed(hash string, elapsed time.Duration) {
	database.Update([]byte(hash+"elapsed"), []byte(elapsed.String()))
}

// GetElapsed
// Gets the elapsed time from the database
func GetElapsed(hash string) time.Duration {
	data := database.Read([]byte(hash + "elapsed"))
	parsedTime, _ := time.ParseDuration(string(data))
	return parsedTime
}

// GetStatus
// Gets the status from the database
func GetStatus(seed string) Message {
	// Simply get the base seed status - no loops or aggregation
	data := database.Read([]byte("Stats" + seed))
	var message Message
	if len(data) > 0 {
		err := json.Unmarshal([]byte(string(data)), &message)
		if err != nil {
			fmt.Println("Error decoding JSON:", err)
			return Message{Status: "Pending"}
		}
	}
	return message
}

// GetNodeStatus
// Gets the status for a specific node
func GetNodeStatus(seed string, nodeName string) Message {
	data := database.Read([]byte("Stats" + seed + ":" + nodeName))
	var message Message
	if len(data) > 0 {
		err := json.Unmarshal([]byte(string(data)), &message)
		if err != nil {
			fmt.Println("Error decoding JSON:", err)
		}
	}
	return message
}

// GetAllNodeStatuses
// Gets all node statuses for a given seed, useful for reporting to honeycomb
func GetAllNodeStatuses(seed string) []Message {
	var statuses []Message
	// Limit iteration to prevent blocking
	maxNodes := 20
	count := 0
	for _, nodeName := range PeerNames {
		if count >= maxNodes {
			break
		}
		nodeData := database.Read([]byte("Stats" + seed + ":" + nodeName))
		if len(nodeData) > 0 {
			var message Message
			err := json.Unmarshal([]byte(string(nodeData)), &message)
			if err == nil && message.Status != "" && message.Status != "Pending" {
				statuses = append(statuses, message)
			}
		}
		count++
	}
	return statuses
}

// SetStatus
// Sets the status to the database
func SetStatus(seed string, cid string, status string, name string) {
	// fmt.Println("SetStatus", seed, cid, status)
	time1 := GetTime(seed)
	
	// Get the appropriate elapsed time
	var elapsed time.Duration
	if status == "Pending" {
		// For initial status, use base seed
		elapsed = GetElapsed(seed)
	} else {
		// For proof responses, use node-specific key to avoid conflicts
		nodeKey := seed + ":" + name
		elapsed = GetElapsed(nodeKey)
	}
	
	timeString := time1.Format(time.RFC3339)
	elapsedString := elapsed.String()
	jsonString := `{"type": "ProofOfAccess", "CID":"` + cid + `", "seed":"` + seed + `", "status":"` + status + `", "name":"` + name + `", "time":"` + timeString + `", "elapsed":"` + elapsedString + `"}`
	jsonString = strings.TrimSpace(jsonString)
	
	// Always store in base location for backward compatibility
	database.Update([]byte("Stats"+seed), []byte(jsonString))
	
	// Additionally store node-specific for tracking individual responses
	if status != "Pending" {
		database.Update([]byte("Stats"+seed+":"+name), []byte(jsonString))
	}
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
	for _, peerName := range PeerNames {
		//fmt.Println("Peer: ", peerName)
		//fmt.Println("Size: ", PeerSize[peerName])
		NetworkStorage = NetworkStorage + PeerSize[peerName]
	}
	NetworkStorage = NetworkStorage / 1024 / 1024 / 1024
	peers := len(PeerNames)

	record := NetworkRecord{
		Peers:          peers,
		NetworkStorage: NetworkStorage,
		Date:           currentTime,
	}

	jsonRecord, err := json.Marshal(record)
	fmt.Println("JSON Record: ", string(jsonRecord))
	if err != nil {
		fmt.Println("Error encoding JSON")
		return
	}

	database.Save([]byte("Network"+currentTime), jsonRecord)
}
