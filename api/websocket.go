package api

import (
	"net/http"
	"proofofaccess/localdata"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type ExampleResponse struct {
	Status  string `json:"Status"`
	Message string `json:"Message"`
	Elapsed string `json:"Elapsed"`
	// Add context fields for honeycomb-spkcc compatibility
	Name    string `json:"Name,omitempty"`
	CID     string `json:"CID,omitempty"`
	Bn      int    `json:"bn,omitempty"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type message struct {
	// Support both uppercase and lowercase field names for compatibility
	Name   string `json:"name"`
	NameUC string `json:"Name"` // Uppercase variant
	CID    string `json:"cid"`
	CIDUC  string `json:"CID"` // Uppercase variant
	SALT   string `json:"salt"`
	SALTUC string `json:"SALT"` // Uppercase variant
	PEERID string `json:"peerid"`
	Page   int    `json:"page"`
	User   string `json:"username"`
	Bn     int    `json:"bn,omitempty"` // Add block number for context
	
	// Additional fields from honeycomb-spkcc
	Timestamp int64  `json:"timestamp,omitempty"`
	Validator string `json:"validator,omitempty"`
}

// Normalize message fields to handle both uppercase and lowercase
func (m *message) Normalize() {
	if m.Name == "" && m.NameUC != "" {
		m.Name = m.NameUC
	}
	if m.CID == "" && m.CIDUC != "" {
		m.CID = m.CIDUC
	}
	if m.SALT == "" && m.SALTUC != "" {
		m.SALT = m.SALTUC
	}
}

// BatchRequest handles batch validation requests from honeycomb-spkcc
type BatchRequest struct {
	Type        string     `json:"type"`
	Validations []message  `json:"validations"`
}

// BatchResponse for batch validation results
type BatchResponse struct {
	Type    string             `json:"type"`
	Results []ExampleResponse  `json:"results"`
}

var wsMutex = &sync.Mutex{}

func upgradeToWebSocket(c *gin.Context) *websocket.Conn {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	//fmt.Println("upgradeToWebSocket")
	if err != nil {
		log.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upgrade connection"})
		return nil
	}
	//fmt.Println("upgradeToWebSocket2")

	return conn
}

func closeWebSocket(conn *websocket.Conn) {
	// log.Info("Closing WebSocket connection")
	err := conn.Close()
	if err != nil {
		return
	}
}

func readWebSocketMessage(conn *websocket.Conn) (*message, error) {
	var msg message
	if err := conn.ReadJSON(&msg); err != nil {
		log.Error(err)
		sendWsResponse(wsError, "Failed to read JSON from WebSocket connection", "0", conn)
		return nil, err
	}

	return &msg, nil
}

// Track closed connections to avoid repeated write attempts
var closedConnections = make(map[*websocket.Conn]bool)
var closedConnMutex sync.Mutex

func sendWsResponse(status string, message string, elapsed string, conn *websocket.Conn) {
	// Check if connection is already known to be closed
	closedConnMutex.Lock()
	if closedConnections[conn] {
		closedConnMutex.Unlock()
		return
	}
	closedConnMutex.Unlock()
	
	localdata.Lock.Lock()
	err := conn.WriteJSON(ExampleResponse{
		Status:  status,
		Message: message,
		Elapsed: elapsed,
	})
	localdata.Lock.Unlock()
	
	if err != nil {
		// Mark connection as closed and log only once
		closedConnMutex.Lock()
		if !closedConnections[conn] {
			closedConnections[conn] = true
			// Only log the first error for this connection
			log.Println("WebSocket connection closed:", err)
		}
		closedConnMutex.Unlock()
	}
}

// sendWsResponseWithContext sends a response with additional context for honeycomb-spkcc
func sendWsResponseWithContext(status string, message string, elapsed string, name string, cid string, bn int, conn *websocket.Conn) error {
	// Check if connection is already known to be closed
	closedConnMutex.Lock()
	if closedConnections[conn] {
		closedConnMutex.Unlock()
		return websocket.ErrCloseSent
	}
	closedConnMutex.Unlock()
	
	localdata.Lock.Lock()
	err := conn.WriteJSON(ExampleResponse{
		Status:  status,
		Message: message,
		Elapsed: elapsed,
		Name:    name,
		CID:     cid,
		Bn:      bn,
	})
	localdata.Lock.Unlock()
	
	if err != nil {
		// Mark connection as closed and log only once
		closedConnMutex.Lock()
		if !closedConnections[conn] {
			closedConnections[conn] = true
			// Only log the first error for this connection
			log.Println("WebSocket connection closed:", err)
		}
		closedConnMutex.Unlock()
		return err
	}
	return nil
}

func sendJsonWS(conn *websocket.Conn, json gin.H) {
	err := conn.WriteJSON(json)
	if err != nil {
		return
	}
}
