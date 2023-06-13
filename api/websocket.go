package api

import (
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"net/http"
)

type ExampleResponse struct {
	Status  string `json:"Status"`
	Message string `json:"Message"`
	Elapsed string `json:"Elapsed"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type message struct {
	Name   string `json:"name"`
	CID    string `json:"cid"`
	SALT   string `json:"salt"`
	PEERID string `json:"peerid"`
}

func upgradeToWebSocket(c *gin.Context) (*websocket.Conn, error) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upgrade connection"})
		return nil, err
	}

	return conn, nil
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
	err := conn.WriteJSON(gin.H{
		"Status":  status,
		"Message": message,
		"Elapsed": elapsed,
	})
	if err != nil {
		return
	}
}
func sendJsonWS(conn *websocket.Conn, json gin.H) {
	err := conn.WriteJSON(json)
	if err != nil {
		return
	}
}
