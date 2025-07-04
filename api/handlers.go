package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"sort"
	"sync"
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

	// Fetch stats from in-memory storage
	stats := localdata.GetStats(key)

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
	stats := localdata.GetNetwork()

	// Sort stats by date
	sort.Slice(stats, func(i, j int) bool {
		timeI, errI := time.Parse(time.RFC3339, stats[i].Date)
		timeJ, errJ := time.Parse(time.RFC3339, stats[j].Date)

		if errI != nil || errJ != nil {
			logrus.Warn("Error parsing time in network stats for sorting")
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
	// Log when the endpoint is hit
	logrus.Infof("=== /validate endpoint hit from %s ===", c.ClientIP())

	conn := upgradeToWebSocket(c)
	if conn == nil {
		logrus.Errorf("Failed to upgrade to WebSocket for validation request from %s", c.ClientIP())
		return
	}
	defer closeWebSocket(conn)

	// Keep connection open for batch processing
	for {
		var rawMsg json.RawMessage
		err := conn.ReadJSON(&rawMsg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logrus.Errorf("WebSocket error: %v", err)
			}
			break
		}

		// Check if it's a batch request
		var batchCheck struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(rawMsg, &batchCheck); err == nil && batchCheck.Type == "batch" {
			// Handle batch request
			var batchReq struct {
				Type        string                `json:"type"`
				Validations []messaging.Request `json:"validations"`
			}
			if err := json.Unmarshal(rawMsg, &batchReq); err != nil {
				logrus.Errorf("Failed to parse batch request: %v", err)
				continue
			}

			logrus.Infof("Batch validation request received with %d validations", len(batchReq.Validations))

			// Process each validation in parallel
			results := make([]map[string]interface{}, len(batchReq.Validations))
			var wg sync.WaitGroup
			
			for i, req := range batchReq.Validations {
				wg.Add(1)
				go func(idx int, request messaging.Request) {
					defer wg.Done()
					result := processValidation(request, conn)
					results[idx] = result
				}(i, req)
			}
			
			// Wait for all validations to complete
			wg.Wait()

			// Send batch response
			batchResponse := map[string]interface{}{
				"type":    "batch",
				"results": results,
			}
			if err := conn.WriteJSON(batchResponse); err != nil {
				logrus.Errorf("Failed to send batch response: %v", err)
			}
		} else {
			// Handle single request (backwards compatibility)
			var msg messaging.Request
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				logrus.Errorf("Failed to parse single request: %v", err)
				continue
			}

			logrus.Infof("Single validation request received - Name: %s, CID: %s, SALT: %s, PEERID: %s", msg.Name, msg.CID, msg.SALT, msg.PEERID)
			
			// Process single validation
			go processSingleValidation(msg, conn)
		}
	}
}

// Process a single validation and return result
func processValidation(msg messaging.Request, conn *websocket.Conn) map[string]interface{} {
	result := map[string]interface{}{
		"Name":   msg.Name,
		"CID":    msg.CID,
		"bn":     msg.Bn,
		"Status": "Processing",
	}

	// Create a channel to receive the final status
	statusChan := make(chan map[string]interface{}, 1)
	
	// Process validation in goroutine
	go func() {
		// Get peer ID
		peerID, err := getPeerID(msg, conn)
		if err != nil {
			statusChan <- map[string]interface{}{
				"Name":   msg.Name,
				"CID":    msg.CID,
				"bn":     msg.Bn,
				"Status": "IpfsPeerIDError",
				"Error":  err.Error(),
			}
			return
		}

		// Attempt connection
		err = connectToPeer(peerID, conn, msg)
		if err != nil {
			logrus.Warnf("Direct IPFS connection to peer %s failed: %v", peerID, err)
		}

		// Prepare salt
		salt := msg.SALT
		if salt == "" {
			salt, err = createRandomHash(conn)
			if err != nil {
				statusChan <- map[string]interface{}{
					"Name":   msg.Name,
					"CID":    msg.CID,
					"bn":     msg.Bn,
					"Status": "Error",
					"Error":  "Failed to create salt",
				}
				return
			}
		}

		// Create and send proof request
		proofJson, err := createProofRequest(salt, msg.CID, conn, msg.Name)
		if err != nil {
			statusChan <- map[string]interface{}{
				"Name":   msg.Name,
				"CID":    msg.CID,
				"bn":     msg.Bn,
				"Status": "Error",
				"Error":  "Failed to create proof request",
			}
			return
		}

		err = sendProofRequest(salt, msg.CID, proofJson, msg.Name, conn)
		if err != nil {
			statusChan <- map[string]interface{}{
				"Name":   msg.Name,
				"CID":    msg.CID,
				"bn":     msg.Bn,
				"Status": "Error",
				"Error":  "Failed to send proof request",
			}
			return
		}

		// Wait for validation result with timeout
		timeout := time.After(30 * time.Second)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-timeout:
				statusChan <- map[string]interface{}{
					"Name":   msg.Name,
					"CID":    msg.CID,
					"bn":     msg.Bn,
					"Status": "Timeout",
				}
				return
			case <-ticker.C:
				// Check if consensus is complete
				consensusValue := localdata.GetConsensus(msg.CID, salt)
				if consensusValue != "" {
					elapsed := localdata.GetElapsed(msg.CID, salt)
					statusChan <- map[string]interface{}{
						"Name":    msg.Name,
						"CID":     msg.CID,
						"bn":      msg.Bn,
						"Status":  consensusValue,
						"Elapsed": elapsed,
					}
					return
				}
			}
		}
	}()

	// Wait for result with timeout
	select {
	case finalResult := <-statusChan:
		return finalResult
	case <-time.After(35 * time.Second): // Slightly longer than validation timeout
		return map[string]interface{}{
			"Name":   msg.Name,
			"CID":    msg.CID,
			"bn":     msg.Bn,
			"Status": "Timeout",
		}
	}
}

// Process single validation (original logic)
func processSingleValidation(msg messaging.Request, conn *websocket.Conn) {
	logrus.Infof("Processing single validation - Name: %s, CID: %s, SALT: %s, PEERID: %s", msg.Name, msg.CID, msg.SALT, msg.PEERID)

	// --- Peer Discovery/Connection ---
	peerID, err := getPeerID(msg, conn)
	if err != nil {
		logrus.Errorf("Failed to get peer ID for validation request: %v", err)
		return
	}
	logrus.Infof("Peer ID obtained: %s for user: %s", peerID, msg.Name)

	// Attempt IPFS peer connection (optional - validation can proceed without it)
	err = connectToPeer(peerID, conn, msg) // Ping check
	if err != nil {
		logrus.Warnf("Direct IPFS connection to peer %s (user: %s) failed: %v", peerID, msg.Name, err)
		logrus.Infof("Continuing with proof validation via PubSub/WebSocket for user: %s", msg.Name)
		// Don't return - continue with proof validation
	} else {
		logrus.Infof("Successfully connected to peer %s (user: %s)", peerID, msg.Name)
	}

	// --- Prepare Proof Request ---
	salt := msg.SALT
	if salt == "" {
		salt, err = createRandomHash(conn)
		if err != nil {
			logrus.Errorf("Failed to create random hash: %v", err)
			return
		}
		logrus.Debugf("Generated random salt: %s", salt)
	}
	CID := msg.CID
	// Use username for targeting (PubSub compatibility) but track peer ID
	targetUsername := msg.Name

	logrus.Infof("Starting proof validation for CID: %s, target user: %s, peer: %s, salt: %s", CID, targetUsername, peerID, salt)

	// Set initial status to Pending - use username for compatibility
	proofJson, err := createProofRequest(salt, CID, conn, targetUsername)
	if err != nil {
		logrus.Errorf("Failed to create proof request: %v", err)
		return
	}

	// --- Send Request & Schedule Consensus Check ---
	logrus.Infof("Sending proof request to user %s (peer: %s) for CID %s", targetUsername, peerID, CID)
	err = sendProofRequest(salt, CID, proofJson, targetUsername, conn) // Use username for targeting
	if err != nil {
		logrus.Errorf("Failed to send proof request: %v", err)
		return
	}

	startTime := localdata.GetTime(CID, salt) // Retrieve the time request was sent
	if startTime.IsZero() {
		logrus.Errorf("Failed to retrieve start time for CID %s, salt %s after sending request.", CID, salt)
		sendWsResponse(wsError, "Internal error: Cannot track request time", "0", conn)
		return
	}

	// Define timeout duration (should be consistent)
	validationTimeoutDuration := 30 * time.Second // Increased from 10s to 30s for testing

	// Mark that consensus *should* run for this key, but hasn't started yet
	messaging.ConsensusProcessingMutex.Lock() // Use exported mutex
	messaging.ConsensusProcessing[CID+salt] = false
	messaging.ConsensusProcessingMutex.Unlock()

	// Schedule the consensus processing function to run after the timeout
	time.AfterFunc(validationTimeoutDuration, func() {
		// This function will run after the timeout duration
		// Pass targetName to ProcessProofConsensus (exported function)
		logrus.Debugf("Consensus timeout reached, triggering consensus check for CID %s, salt %s", CID, salt)
		messaging.ProcessProofConsensus(CID, salt, targetUsername, startTime)
	})
	logrus.Infof("Scheduled consensus check for CID %s, salt %s in %v", CID, salt, validationTimeoutDuration)

	// --- Wait for and Report Final Status (Polling) ---
	sendWsResponse("Processing", "Waiting for validation consensus", "0", conn)
	logrus.Infof("Waiting for consensus result for CID %s...", CID)

	pollTicker := time.NewTicker(1 * time.Second)
	defer pollTicker.Stop()
	// Wait slightly longer than the validation timeout for consensus function to complete
	pollTimeout := time.After(validationTimeoutDuration + 5*time.Second) // Increased buffer to 5s

	finalStatus := "Timeout"       // Default status if polling times out
	var finalElapsed time.Duration // Use to store the elapsed time from the status message

	for {
		select {
		case <-pollTicker.C:
			// Check the status record (keyed by seed only)
			statusMsg := localdata.GetStatus(salt)
			// Check for non-empty, non-pending status
			if statusMsg.Status != "" && statusMsg.Status != "Pending" {
				// Consensus process has finished and set the status
				finalStatus = statusMsg.Status
				// Use the elapsed time from the consensus result if available
				logrus.Infof("Retrieved status message - Status: %s, Elapsed: '%s', Name: %s", statusMsg.Status, statusMsg.Elapsed, statusMsg.Name)
				// Use elapsed time from consensus unless it's completely empty
				if statusMsg.Elapsed != "" {
					if parsed, err := time.ParseDuration(statusMsg.Elapsed); err == nil {
						finalElapsed = parsed
						logrus.Infof("Using elapsed time from consensus: %v", finalElapsed)
					} else {
						// Fallback to calculated time
						finalElapsed = time.Since(startTime)
						logrus.Warnf("Failed to parse elapsed time '%s': %v, using calculated time: %v", statusMsg.Elapsed, err, finalElapsed)
					}
				} else {
					finalElapsed = time.Since(startTime)
					logrus.Warnf("No elapsed time in consensus result (was: '%s'), using calculated time: %v", statusMsg.Elapsed, finalElapsed)
				}
				logrus.Infof("Consensus result received for CID %s, salt %s: %s (took %v)", CID, salt, finalStatus, finalElapsed)
				goto reportResult // Exit loop
			}
		case <-pollTimeout:
			logrus.Warnf("Polling timeout waiting for consensus result for CID %s, salt %s", CID, salt)
			// Check status one last time in case it finished right at the timeout
			statusMsg := localdata.GetStatus(salt)
			if statusMsg.Status != "" && statusMsg.Status != "Pending" {
				finalStatus = statusMsg.Status
				// Use the elapsed time from the consensus result if available
				if statusMsg.Elapsed != "" {
					if parsed, err := time.ParseDuration(statusMsg.Elapsed); err == nil {
						finalElapsed = parsed
					} else {
						// Fallback to calculated time
						finalElapsed = time.Since(startTime)
					}
				} else {
					finalElapsed = time.Since(startTime)
				}
				logrus.Infof("Final consensus result (at timeout) for CID %s: %s (took %v)", CID, finalStatus, finalElapsed)
			} else {
				// Consensus didn't finish or set status in time.
				finalStatus = "Timeout"                                  // Or "Invalid"
				finalElapsed = validationTimeoutDuration + 5*time.Second // Report polling timeout duration
				// Check if consensus processing was ever started (it should have been by AfterFunc)
				messaging.ConsensusProcessingMutex.Lock() // Use exported mutex
				processing := messaging.ConsensusProcessing[CID+salt]
				messaging.ConsensusProcessingMutex.Unlock()
				if !processing {
					// This case should ideally not happen if AfterFunc works correctly
					logrus.Errorf("Consensus for %s was not triggered by AfterFunc within timeout!", CID+salt)
					// Manually trigger as a fallback, though the state might be inconsistent
					go messaging.ProcessProofConsensus(CID, salt, targetUsername, startTime)
				}
			}
			goto reportResult // Exit loop
		}
	}

reportResult:
	// Report the final status determined by consensus or timeout
	logrus.Infof("=== Validation complete for CID %s - Final result: %s (took %v) ===", CID, finalStatus, finalElapsed)
	sendWsResponse(finalStatus, finalStatus, formatElapsed(finalElapsed), conn)
	// Gin handlers do not have return values
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
				logrus.Debugf("WebSocket closed for user %s: %v", clientUser, err)
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
	// Revert to original node type check logic
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
