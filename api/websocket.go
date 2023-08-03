package api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"net/http"
	"proofofaccess/localdata"
	"sync"
)

type ExampleResponse struct {
	Status  string `json:"Status"`
	Message string `json:"Message"`
	Elapsed string `json:"Elapsed"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type message struct {
	Name   string `json:"name"`
	CID    string `json:"cid"`
	SALT   string `json:"salt"`
	PEERID string `json:"peerid"`
	Page   int    `json:"page"`
}

var wsMutex = &sync.Mutex{}

func upgradeToWebSocket(c *gin.Context) *websocket.Conn {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	fmt.Println("upgradeToWebSocket")
	if err != nil {
		log.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upgrade connection"})
		return nil
	}
	fmt.Println("upgradeToWebSocket2")

	return conn
}

func closeWebSocket(conn *websocket.Conn) {
	log.Info("Closing WebSocket connection")
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

func sendWsResponse(status string, message string, elapsed string, conn *websocket.Conn) {
	localdata.Lock.Lock()
	err := conn.WriteJSON(ExampleResponse{
		Status:  status,
		Message: message,
		Elapsed: elapsed,
	})
	localdata.Lock.Unlock()
	if err != nil {
		log.Println("Error writing JSON to websocket:", err)
	}
}

func sendJsonWS(conn *websocket.Conn, json gin.H) {
	err := conn.WriteJSON(json)
	if err != nil {
		return
	}
}
