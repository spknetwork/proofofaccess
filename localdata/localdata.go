package localdata

import (
	"encoding/json"
	"fmt"
	"proofofaccess/database"
	"strings"
	"time"
)

var Time = time.Now()

const Layout = "2006-01-02 15:04:05.999999 -0700 MST m=+0.000000000"

type Message struct {
	Type   string `json:"type"`
	Hash   string `json:"hash"`
	CID    string `json:"CID"`
	Status string `json:"status"`
}

var NodeName = ""

// SaveTime
// Saves the time to the database
func SaveTime(hash string) {
	fmt.Println("Saving time to database")
	Time = time.Now()
	timeStr := Time.Format(Layout)
	fmt.Println("Time:", timeStr)
	database.Update([]byte(hash+"time"), []byte(timeStr))
	fmt.Println("Time saved to database")
}

// GetTime
// Gets the time from the database
func GetTime(hash string) time.Time {
	data := database.Read([]byte(hash + "time"))
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
func GetStatus(hash string) string {
	data := database.Read([]byte(hash))
	var message Message
	err := json.Unmarshal([]byte(string(data)), &message)
	if err != nil {
		fmt.Println("Error decoding JSON:", err)
	}
	return message.Status
}

// SetStatus
// Sets the status to the database
func SetStatus(seed string, hash string, status string) {

	jsonString := `{"type": "ProofOfAccess", "hash":"` + hash + `", "seed":"` + seed + `", "status":"` + status + `"}`
	jsonString = strings.TrimSpace(jsonString)
	database.Update([]byte(seed), []byte(jsonString))
}

// SetNodeName
// Sets the node name in localdata
func SetNodeName(name string) {
	NodeName = name
}

func GetNodeName() string {
	return NodeName
}
