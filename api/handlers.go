package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"proofofaccess/database"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

var WsClients = make(map[string]*websocket.Conn)
var broadcast = make(chan messaging.Request)

type WSMessage struct {
	Body   string `json:"body"`
	Sender string `json:"sender"`
}

func getStatsHandler(c *gin.Context) {
	conn := upgradeToWebSocket(c)
	if conn == nil {
		logrus.Error("Failed to upgrade getStatsHandler to WebSocket")
		return
	}
	defer closeWebSocket(conn)
	msg, err := readWebSocketMessage(conn)

	page := msg.Page
	key := msg.User
	if err != nil {
		logrus.Errorf("Error reading WebSocket message in getStatsHandler: %v", err)
		return
	}

	const pageSize = 50 // define the number of results per page

	// Fetch stats from the database
	stats := database.GetStats(key)

	// Sort stats by date
	sort.Slice(stats, func(i, j int) bool {
		timeI, errI := stats[i].Time, error(nil)
		timeJ, errJ := stats[j].Time, error(nil)

		if errI != nil || errJ != nil {
			logrus.Warn("Error parsing time in stats for sorting (this should not happen if type is time.Time)")
			return false
		}

		return timeI.After(timeJ)
	})

	// Apply pagination
	startIndex := (page - 1) * pageSize
	if startIndex >= len(stats) {
		logrus.Warnf("Page number %d out of range for stats (key: %s, total: %d)", page, key, len(stats))
		sendWsResponse("Error", "Page number out of range", "0", conn)
		return
	}
	endIndex := startIndex + pageSize
	if endIndex > len(stats) {
		endIndex = len(stats)
	}
	pagedStats := stats[startIndex:endIndex]

	// Convert pagedStats to JSON string
	statsJson, err := json.Marshal(pagedStats)
	if err != nil {
		logrus.Errorf("Error encoding paged stats to JSON: %v", err)
		return
	}

	sendWsResponse("OK", string(statsJson), "0", conn)
}

func getNetworkHandler(c *gin.Context) {
	conn := upgradeToWebSocket(c)
	if conn == nil {
		logrus.Error("Failed to upgrade getNetworkHandler to WebSocket")
		return
	}
	defer closeWebSocket(conn)
	msg, err := readWebSocketMessage(conn)

	page := msg.Page
	if err != nil {
		logrus.Errorf("Error reading WebSocket message in getNetworkHandler: %v", err)
		return
	}

	const pageSize = 4000 // define the number of results per page
	stats := database.GetNetwork()

	// Sort stats by date
	sort.Slice(stats, func(i, j int) bool {
		timeI, errI := stats[i].Date, error(nil)
		timeJ, errJ := stats[j].Date, error(nil)

		if errI != nil || errJ != nil {
			logrus.Warn("Error parsing time in network stats for sorting (this should not happen if type is time.Time)")
			return false
		}

		return timeI.After(timeJ)
	})

	// Apply pagination
	startIndex := (page - 1) * pageSize
	if startIndex >= len(stats) {
		logrus.Warnf("Page number %d out of range for network stats (total: %d)", page, len(stats))
		sendWsResponse("Error", "Page number out of range", "0", conn)
		return
	}
	endIndex := startIndex + pageSize
	if endIndex > len(stats) {
		endIndex = len(stats)
	}
	pagedStats := stats[startIndex:endIndex]

	// Convert pagedStats to JSON string
	statsJson, err := json.Marshal(pagedStats)
	if err != nil {
		logrus.Errorf("Error encoding paged network stats to JSON: %v", err)
		return
	}

	sendWsResponse("OK", string(statsJson), "0", conn)
}

func handleValidate(c *gin.Context) {
	conn := upgradeToWebSocket(c)
	if conn == nil {
		return
	}
	defer closeWebSocket(conn)

	msg, err := readWebSocketMessage(conn)
	if err != nil {
		return
	}

	// --- Peer Discovery/Connection ---
	peerID, err := getPeerID(msg, conn)
	if err != nil {
		return
	}
	err = connectToPeer(peerID, conn, msg) // Ping check
	if err != nil {
		return
	}

	// --- Prepare Proof Request ---
	salt := msg.SALT
	if salt == "" {
		salt, err = createRandomHash(conn)
		if err != nil {
			return
		}
	}
	CID := msg.CID
	targetName := msg.Name // Get target name from message

	// Set initial status to Pending (createProofRequest does this indirectly via SetStatus)
	proofJson, err := createProofRequest(salt, CID, conn, targetName)
	if err != nil {
		return
	}

	// --- Send Request & Start Consensus Timer ---
	// Pass CID to sendProofRequest
	err = sendProofRequest(salt, CID, proofJson, targetName, conn) // Saves request time using CID+salt
	if err != nil {
		return
	}

	startTime := localdata.GetTime(CID, salt) // Retrieve the time request was sent
	if startTime.IsZero() {
		logrus.Errorf("Failed to retrieve start time for CID %s, salt %s after sending request.", CID, salt)
		sendWsResponse(wsError, "Internal error: Cannot track request time", "0", conn)
		return
	}

	// Define timeout duration (should be consistent)
	validationTimeoutDuration := 10 * time.Second // e.g., 10 seconds

	// Mark that consensus *should* run for this key, but hasn't started yet
	messaging.ConsensusProcessingMutex.Lock() // Use exported mutex
	messaging.ConsensusProcessing[CID+salt] = false
	messaging.ConsensusProcessingMutex.Unlock()

	logrus.Infof("Proof request sent for CID %s, salt %s to %s. Starting %v timer for consensus.", CID, salt, targetName, validationTimeoutDuration)
	time.AfterFunc(validationTimeoutDuration, func() {
		// This function will run after the timeout duration
		// Pass targetName to ProcessProofConsensus (exported function)
		messaging.ProcessProofConsensus(CID, salt, targetName, startTime)
	})

	// --- Wait for and Report Final Status (Polling) ---
	sendWsResponse("Processing", "Waiting for validation consensus", "0", conn)
	pollTicker := time.NewTicker(1 * time.Second)
	defer pollTicker.Stop()
	// Wait slightly longer than the validation timeout for consensus function to complete
	pollTimeout := time.After(validationTimeoutDuration + 3*time.Second) // Increased buffer slightly

	finalStatus := "Timeout"       // Default status if polling times out
	var finalElapsed time.Duration // We may not have a meaningful single elapsed time now

	for {
		select {
		case <-pollTicker.C:
			// Check the status record (keyed by seed only)
			statusMsg := localdata.GetStatus(salt)
			// Check for non-empty, non-pending status
			if statusMsg.Status != "" && statusMsg.Status != "Pending" {
				// Consensus process has finished and set the status
				finalStatus = statusMsg.Status
				// Elapsed time from the status record might not be relevant for consensus. Report 0?
				logrus.Infof("Consensus result received for salt %s: %s", salt, finalStatus)
				goto reportResult // Exit loop
			}
		case <-pollTimeout:
			logrus.Warnf("Polling timeout waiting for consensus result for salt %s", salt)
			// Check status one last time in case it finished right at the timeout
			statusMsg := localdata.GetStatus(salt)
			if statusMsg.Status != "" && statusMsg.Status != "Pending" {
				finalStatus = statusMsg.Status
			} else {
				// Consensus didn't finish or set status in time.
				finalStatus = "Timeout"                   // Or "Invalid"
				messaging.ConsensusProcessingMutex.Lock() // Use exported mutex
				processing := messaging.ConsensusProcessing[CID+salt]
				messaging.ConsensusProcessingMutex.Unlock()
				if !processing {
					logrus.Warnf("Consensus for %s not started by timeout, triggering manually.", CID+salt)
					// Call exported function
					go messaging.ProcessProofConsensus(CID, salt, targetName, startTime) // Run in background
					time.Sleep(100 * time.Millisecond)                                   // Small delay
					statusMsg = localdata.GetStatus(salt)                                // Check again
					if statusMsg.Status != "" && statusMsg.Status != "Pending" {
						finalStatus = statusMsg.Status
					}
				}

			}
			goto reportResult // Exit loop
		}
	}

reportResult:
	// Report the final status determined by consensus or timeout
	sendWsResponse(finalStatus, finalStatus, formatElapsed(finalElapsed), conn)
}

func handleMessaging(c *gin.Context) {
	ws := upgradeToWebSocket(c)
	if ws == nil {
		logrus.Error("Failed to upgrade handleMessaging to WebSocket")
		return
	}

	defer ws.Close()

	clientUser := ""
	for {
		var msg messaging.Request
		err := ws.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logrus.Errorf("Error reading JSON from WebSocket: %v", err)
			} else {
				logrus.Warnf("WebSocket closed for user %s: %v", clientUser, err)
			}
			if clientUser != "" {
				if _, ok := localdata.WsClients[clientUser]; ok {
					delete(localdata.WsClients, clientUser)
				}
			}
			break
		}

		if clientUser == "" && msg.User != "" {
			clientUser = msg.User
		}

		if localdata.WsPeers[msg.User] != msg.User || localdata.WsClients[msg.User] != ws {
			logrus.Debugf("Registering WebSocket client: %s", msg.User)
			localdata.Lock.Lock()
			localdata.WsClients[msg.User] = ws
			localdata.WsPeers[msg.User] = msg.User
			localdata.Lock.Unlock()
		}
		jsonData, err := json.Marshal(msg)
		if err != nil {
			logrus.Errorf("Error encoding received message back to JSON: %v", err)
			continue
		}
		go messaging.HandleMessage(string(jsonData), ws)
	}
}

func handleShutdown(c *gin.Context) {
	if localdata.NodeType != 1 {

		logrus.Info("Shutdown request received via API. Preparing to shut down the application...")

		// Respond to the client
		c.JSON(http.StatusOK, gin.H{
			"message": "Shutdown request received. The application will shut down.",
		})

		// Use a goroutine to shut down the application after responding to the client
		go func() {
			// Wait a bit to make sure the response can be sent before the application shuts down
			time.Sleep(1 * time.Second)
			logrus.Info("Shutting down the application now...")
			os.Exit(0)
		}()
	} else {
		logrus.Warn("Shutdown request received via API, but ignored (Node Type is 1)")
		c.JSON(http.StatusForbidden, gin.H{
			"message": "Shutdown is not allowed for this node type.",
		})
	}
}

func handleStats(c *gin.Context) {
	conn := upgradeToWebSocket(c)
	key := c.Query("username")
	if conn == nil {
		logrus.Error("Failed to upgrade to WebSocket")
		return
	}
	defer closeWebSocket(conn)
	stats(conn, key)
	return
}

func getCIDHandler(c *gin.Context) {
	key := c.Query("key")
	localdata.Lock.Lock()
	percentage := localdata.CIDRefPercentage[key]
	status := localdata.CIDRefStatus[key]
	localdata.Lock.Unlock()
	c.JSON(http.StatusOK, gin.H{
		"CID":        key,
		"percentage": percentage,
		"status":     status,
	})
}

func getPeerHandler(c *gin.Context) {
	key := c.Query("username")
	cids := localdata.PeerCids[key]
	c.JSON(http.StatusOK, gin.H{
		"CIDs": cids,
	})
}

func getIPHandler(c *gin.Context) {
	domain := c.Query("domain")
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain parameter is required"})
		return
	}

	ip, err := getIP(domain)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error fetching IP for %s: %v", domain, err)})
		return
	}

	resp := Response{IP: ip}
	c.JSON(http.StatusOK, resp)
}

func getIP(domain string) (string, error) {
	ips, err := net.LookupIP(domain)
	if err != nil {
		return "", err
	}

	if len(ips) > 0 {
		return ips[0].String(), nil
	}
	return "", fmt.Errorf("No IP found for domain: %s", domain)
}

// Definition for formatElapsed if used within this file
func formatElapsed(elapsed time.Duration) string {
	return fmt.Sprintf("%dms", elapsed.Milliseconds())
}
