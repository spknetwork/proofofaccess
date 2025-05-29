package api

import (
	"net/http"
	"proofofaccess/localdata"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
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
	User   string `json:"username"`
}

var wsMutex = &sync.Mutex{}

func upgradeToWebSocket(c *gin.Context) *websocket.Conn {
	logrus.Debugf("Upgrading connection to WebSocket for client: %s", c.ClientIP())
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	//fmt.Println("upgradeToWebSocket")
	if err != nil {
		logrus.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upgrade connection"})
		return nil
	}
	//fmt.Println("upgradeToWebSocket2")
	logrus.Debugf("WebSocket connection established for client: %s", c.ClientIP())

	return conn
}

func closeWebSocket(conn *websocket.Conn) {
	logrus.Debugf("Closing WebSocket connection")
	// log.Info("Closing WebSocket connection")
	err := conn.Close()
	if err != nil {
		logrus.Debugf("Error closing WebSocket connection: %v", err)
		return
	}
	logrus.Debugf("WebSocket connection closed successfully")
}

func readWebSocketMessage(conn *websocket.Conn) (*message, error) {
	var msg message
	logrus.Debugf("Reading WebSocket message...")
	if err := conn.ReadJSON(&msg); err != nil {
		logrus.Error(err)
		sendWsResponse(wsError, "Failed to read JSON from WebSocket connection", "0", conn)
		return nil, err
	}
	logrus.Debugf("Received WebSocket message: Name=%s, CID=%s, User=%s", msg.Name, msg.CID, msg.User)

	return &msg, nil
}

func sendWsResponse(status string, message string, elapsed string, conn *websocket.Conn) {
	localdata.Lock.Lock()
	//fmt.Println("sendWsResponse", status, message, elapsed)
	logrus.Debugf("Sending WebSocket response: status=%s, message=%s, elapsed=%s", status, message, elapsed)
	err := conn.WriteJSON(ExampleResponse{
		Status:  status,
		Message: message,
		Elapsed: elapsed,
	})
	localdata.Lock.Unlock()
	if err != nil {
		logrus.Error("Error writing JSON to websocket:", err)
	} else {
		logrus.Debugf("WebSocket response sent successfully")
	}
}

func sendJsonWS(conn *websocket.Conn, json gin.H) {
	logrus.Debugf("Sending JSON WebSocket message: %+v", json)
	err := conn.WriteJSON(json)
	if err != nil {
		logrus.Errorf("Error sending JSON WebSocket message: %v", err)
		return
	}
	logrus.Debugf("JSON WebSocket message sent successfully")
}
