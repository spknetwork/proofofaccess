package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"proofofaccess/database"
	"proofofaccess/hive"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"proofofaccess/proofcrypto"
	"proofofaccess/pubsub"
	"proofofaccess/validation"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

type ExampleResponse struct {
	Status  string `json:"Status"`
	Message string `json:"Message"`
	Elapsed string `json:"Elapsed"`
}

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	CID = ""
	log = logrus.New()
)

func StartAPI(ctx context.Context, nodeType int) {
	r := gin.Default()

	// Serve the index.html file on the root route
	r.StaticFile("/", "./public/index.html")

	// Handle the API request
	r.GET("/validate", handleValidate)
	r.POST("/write", handleWrite)
	r.GET("/read", handleRead)
	r.GET("/update", handleUpdate)
	r.GET("/delete", handleDelete)
	r.Static("/public", "./public")

	// Start the server
	server := &http.Server{
		Addr:    ":8001",
		Handler: r,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	<-ctx.Done()

	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctxShutdown); err != nil {
		log.Fatalf("server shutdown failed: %s\n", err)
	}
}

func handleValidate(c *gin.Context) {
	// Upgrade the HTTP connection to a WebSocket connection
	log.Info("Entering handleValidate")
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upgrade connection"})
		return
	}
	defer func() {
		log.Info("Closing WebSocket connection")
		conn.Close()
	}()

	// Read the username and CID from the WebSocket connection
	var msg struct {
		Name   string `json:"name"`
		CID    string `json:"cid"`
		SALT   string `json:"salt"`
		PEERID string `json:"peerid"`
	}
	if err := conn.ReadJSON(&msg); err != nil {
		log.Error(err)
		WsResponse("Error", "Failed to read JSON from WebSocket connection", "0", conn)
		return
	}
	name := msg.Name
	CID = msg.CID
	peerid := msg.PEERID
	log.Info("test")
	if peerid == "" && name != "" {
		WsResponse("FetchingHiveAccount", "Fetching Peer ID from Hive", "0", conn)
		peerid, err = hive.GetIpfsID(name)
		if err != nil {
			WsResponse("IpfsPeerIDError", "Please enable Proof of Access and register your ipfs node to your hive account", "0", conn)
			log.Error(err)
			return
		}
	}
	WsResponse("Connecting", "Connecting to Peer", "0", conn)
	log.Info(peerid)
	// err = ipfs.IpfsPingNode(peerid)
	if err != nil {
		WsResponse("PeerNotFound", "Peer Not Found", "0", conn)
		log.Error(err)
		return
	}
	var salt = msg.SALT
	if salt == "" {
		salt, err = proofcrypto.CreateRandomHash()
		if err != nil {
			WsResponse("Error", "Failed to create random hash", "0", conn)
			log.Error(err)
			return
		}
		log.Info("rand: ", salt)
	}
	i := 0
	timeout := time.NewTicker(1 * time.Second)
	defer timeout.Stop()

	pingDone := make(chan struct{})
	defer close(pingDone)

	go func() {
		for {
			select {
			case <-timeout.C:
				if messaging.Ping[salt] {
					close(pingDone)
					return
				}
				messaging.PingPong(salt, name)
				i++
				if i > 5 {
					WsResponse("Connection Error", "Could not connect to peer try again", "0", conn)
					return
				}
			case <-pingDone:
				return
			}
		}
	}()

	<-pingDone

	log.Info("Connected")
	// Create a random seed hash
	hash, err := proofcrypto.CreateRandomHash()
	if err != nil {
		WsResponse("Error", "Failed to create random seed hash", "0", conn)
		log.Error(err)
		return
	}

	// Create a proof request
	proofJson, err := validation.ProofRequestJson(hash, CID)
	if err != nil {
		WsResponse("Error", "Failed to create proof request JSON", "0", conn)
		log.Error(err)
		return
	}

	// Save the proof request to the database
	database.Save([]byte(hash), []byte(proofJson))

	// Save the proof time
	localdata.SaveTime(hash)
	WsResponse("Requesting", "Requesting Proof", "0", conn)
	// Send the proof request to the storage node
	err = pubsub.Publish(proofJson, name)
	if err != nil {
		WsResponse("Error", "Failed to send proof request to storage node", "0", conn)
		log.Error(err)
		return
	}

	// Create a channel to wait for the proof
	WsResponse("Waiting Proof", "Waiting for Proof", "0", conn)
	proofTimeout := time.NewTicker(30 * time.Millisecond)
	defer proofTimeout.Stop()

	proofDone := make(chan struct{})
	defer close(proofDone)

	go func() {
		for {
			select {
			case <-proofTimeout.C:
				if messaging.ProofRequest[CID] {
					close(proofDone)
					return
				}
			case <-proofDone:
				return
			}
		}
	}()

	<-proofDone

	WsResponse("Validating", "Validating", "0", conn)

	// Wait for the proof to be validated
	statusTimeout := time.NewTicker(30 * time.Millisecond)
	defer statusTimeout.Stop()

	statusDone := make(chan struct{})
	defer close(statusDone)

	go func() {
		for {
			select {
			case <-statusTimeout.C:
				status := localdata.GetStatus(hash)
				if status != "Pending" {
					close(statusDone)
					return
				}
			case <-statusDone:
				return
			}
		}
	}()

	<-statusDone

	// Get the proof status and time elapsed
	status := localdata.GetStatus(hash)
	elapsed := localdata.GetElapsed(hash)

	WsResponse(status, status, strconv.FormatFloat(float64(elapsed.Milliseconds()), 'f', 0, 64)+"ms", conn)
	fmt.Println("Proof Status:", status)
	log.Info("Exiting handleValidate")
}

func handleWrite(c *gin.Context) {
	key := c.PostForm("key")
	value := c.PostForm("value")
	database.Save([]byte(key), []byte(value))
	c.JSON(http.StatusOK, gin.H{
		"message": "Data saved successfully",
	})
}

func handleRead(c *gin.Context) {
	key := c.Query("key")
	value := database.Read([]byte(key))
	c.JSON(http.StatusOK, gin.H{
		"value": string(value),
	})
}

func handleUpdate(c *gin.Context) {
	key := c.Query("key")
	value := c.Query("value")
	database.Update([]byte(key), []byte(value))
	c.JSON(http.StatusOK, gin.H{
		"message": "Data Updated successfully",
	})
}

func handleDelete(c *gin.Context) {
	key := c.Query("key")
	database.Delete([]byte(key))
	c.JSON(http.StatusOK, gin.H{
		"message": "Data Deleted successfully",
	})
}

func WsResponse(Status string, message string, Elapsed string, conn *websocket.Conn) {
	// Create a response struct
	response := ExampleResponse{Status: Status, Message: message, Elapsed: Elapsed}

	// Encode the response as JSON
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Write the JSON response to the WebSocket connection
	conn.WriteMessage(websocket.TextMessage, jsonResponse)
}
