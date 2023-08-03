package api

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"os"
	"proofofaccess/database"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"sort"
	"time"
)

var WsClients = make(map[string]*websocket.Conn)
var broadcast = make(chan messaging.Request)

type WSMessage struct {
	Body   string `json:"body"`
	Sender string `json:"sender"`
}

func getStatsHandler(c *gin.Context) {
	conn, err := upgradeToWebSocket(c)
	if err != nil {
		return
	}
	defer closeWebSocket(conn)
	msg, err := readWebSocketMessage(conn)

	page := msg.Page
	if err != nil {
		log.Println("Error parsing page number:", err)
		return
	}

	const pageSize = 50 // define the number of results per page

	// Fetch stats from the database
	stats := database.GetStats()

	// Sort stats by date
	sort.Slice(stats, func(i, j int) bool {
		timeI, errI := stats[i].Time, err
		timeJ, errJ := stats[j].Time, err

		if errI != nil || errJ != nil {
			log.Println("Error parsing time in Message struct")
			return false
		}

		return timeI.After(timeJ)
	})

	// Apply pagination
	startIndex := (page - 1) * pageSize
	if startIndex >= len(stats) {
		log.Println("Error: page number is out of range")
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
		log.Println("Error encoding stats to JSON:", err)
		return
	}

	sendWsResponse("OK", string(statsJson), "0", conn)
}

func getNetworkHandler(c *gin.Context) {
	conn, err := upgradeToWebSocket(c)
	if err != nil {
		return
	}
	defer closeWebSocket(conn)
	msg, err := readWebSocketMessage(conn)

	page := msg.Page
	if err != nil {
		log.Println("Error parsing page number:", err)
		return
	}

	const pageSize = 50 // define the number of results per page

	// Fetch stats from the database
	stats := database.GetNetwork()

	// Sort stats by date
	sort.Slice(stats, func(i, j int) bool {
		timeI, errI := stats[i].Date, err
		timeJ, errJ := stats[j].Date, err

		if errI != nil || errJ != nil {
			log.Println("Error parsing time in Message struct")
			return false
		}

		return timeI.After(timeJ)
	})

	// Apply pagination
	startIndex := (page - 1) * pageSize
	if startIndex >= len(stats) {
		log.Println("Error: page number is out of range")
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
		log.Println("Error encoding stats to JSON:", err)
		return
	}

	sendWsResponse("OK", string(statsJson), "0", conn)
}

func handleValidate(c *gin.Context) {
	log.Info("Entering handleValidate")
	conn, err := upgradeToWebSocket(c)
	if err != nil {
		return
	}
	defer closeWebSocket(conn)

	msg, err := readWebSocketMessage(conn)
	if err != nil {
		return
	}

	//peerID, err := getPeerID(msg, conn)
	//if err != nil {
	//	return
	//}

	//err = connectToPeer(peerID, conn, msg)
	//if err != nil {
	//	return
	//}

	salt := msg.SALT
	if salt == "" {
		salt, err = createRandomHash(conn)
		if err != nil {
			return
		}
	}
	proofJson, err := createProofRequest(salt, msg.CID, conn, msg.Name)
	if err != nil {
		return
	}

	err = sendProofRequest(salt, proofJson, msg.Name, conn)
	if err != nil {
		return
	}

	cid := msg.CID
	status, elapsed, err := waitForProofStatus(salt, cid, conn)
	if err != nil {
		return
	}
	sendWsResponse(status, status, formatElapsed(elapsed), conn)
	log.Info("Exiting handleValidate")
}

func handleMessaging(c *gin.Context) {
	fmt.Println("Entering handleMessaging")
	ws, _ := upgradeToWebSocket(c)
	fmt.Println("Upgraded to websocket")

	defer ws.Close()

	for {
		var msg messaging.Request
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("Error: %v", err)
			if _, ok := localdata.WsClients[msg.User]; ok {
				delete(localdata.WsClients, msg.User)
			}
			break
		}
		// Add client to the clients map
		if localdata.WsPeers[msg.User] != msg.User || localdata.WsClients[msg.User] != ws {
			fmt.Println("Adding client to the clients map")
			localdata.Lock.Lock()
			localdata.WsClients[msg.User] = ws
			localdata.WsPeers[msg.User] = msg.User
			localdata.Lock.Unlock()
		}
		jsonData, err := json.Marshal(msg)
		if err != nil {
			fmt.Println("Error encoding JSON:", err)
			return
		}
		go messaging.HandleMessage(string(jsonData))
	}
}

func handleShutdown(c *gin.Context) {
	if localdata.NodeType != 1 {

		log.Info("Shutdown request received. Preparing to shut down the application...")

		// Respond to the client
		c.JSON(http.StatusOK, gin.H{
			"message": "Shutdown request received. The application will shut down.",
		})

		// Use a goroutine to shut down the application after responding to the client
		go func() {
			// Wait a bit to make sure the response can be sent before the application shuts down
			time.Sleep(5 * time.Second)
			log.Info("Shutting down the application...")
			os.Exit(0)
		}()
	}
}

func handleStats(c *gin.Context) {
	conn, err := upgradeToWebSocket(c)
	if err != nil {
		return
	}
	defer closeWebSocket(conn)
	log.Info("Entering handleStats")
	stats(conn)
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
