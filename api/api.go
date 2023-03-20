package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"proofofaccess/database"
	"proofofaccess/hive"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"proofofaccess/proofcrypto"
	"proofofaccess/pubsub"
	"proofofaccess/validation"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ExampleResponse
// Define a struct to represent the data we want to return from the API
type ExampleResponse struct {
	Status  string `json:"Status"`
	Message string `json:"Message"`
	Elapsed string `json:"Elapsed"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var CID = ""

// Api
// Starts the API and handles the requests
func Api() {
	// Create a new Gin router
	r := gin.Default()

	// Serve the index.html file on the root route
	r.StaticFile("/", "./public/index.html")

	// Handle the API request
	r.GET("/validate", func(c *gin.Context) {
		// Upgrade the HTTP connection to a WebSocket connection
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer conn.Close()

		// Read the username and CID from the WebSocket connection
		var msg struct {
			Name string `json:"name"`
			CID  string `json:"cid"`
		}
		if err := conn.ReadJSON(&msg); err != nil {
			fmt.Println(err)
			return
		}
		name := msg.Name
		CID = msg.CID
		fmt.Println("test")
		WsResponse("Connecting", "Connecting to Peer", "0", conn)
		ipfsid := hive.GetIpfsID(name)
		if ipfsid == "" {
			WsResponse("IpfsPeerIDError", "Please enable Proof of Access and register your ipfs node to your hive account", "0", conn)
			return
		}

		err = ipfs.IpfsPingNode(ipfsid)
		if err != nil {
			fmt.Println(err)
			return
		}
		rand := proofcrypto.CreateRandomHash()

		i := 0
		for ping := messaging.Ping[rand]; ping == false; ping = messaging.Ping[rand] {
			messaging.PingPong(rand, name)
			if i > 5 {
				WsResponse("Connection Error", "Could not connect to peer try again", "0", conn)
				return
			}
			time.Sleep(1 * time.Second)
			i++
		}

		// Create a random seed hash
		hash := proofcrypto.CreateRandomHash()

		// Create a proof request
		proofJson, _ := validation.ProofRequestJson(hash, CID)

		// Save the proof request to the database
		database.Save([]byte(hash), []byte(proofJson))

		// Save the proof time
		localdata.SaveTime(hash)

		// Send the proof request to the storage node
		pubsub.Publish(proofJson, name)

		// Create a channel to wait for the proof
		WsResponse("Waiting Proof", "Waiting for Proof", "0", conn)
		for proofReq := messaging.ProofRequest[CID]; proofReq == false; proofReq = messaging.ProofRequest[CID] {
			time.Sleep(30 * time.Millisecond)
		}

		WsResponse("Validating", "Validating", "0", conn)
		// Wait for the proof to be validated
		for status := localdata.GetStatus(hash); status == "Pending"; status = localdata.GetStatus(hash) {
			time.Sleep(30 * time.Millisecond)
		}

		// Get the proof status and time elapsed
		status := localdata.GetStatus(hash)
		elapsed := localdata.GetElapsed(hash)

		WsResponse(status, status, strconv.FormatFloat(float64(elapsed.Milliseconds()), 'f', 0, 64)+"ms", conn)
	})
	r.POST("/write", func(c *gin.Context) {
		key := c.PostForm("key")
		value := c.PostForm("value")
		database.Save([]byte(key), []byte(value))
		c.JSON(http.StatusOK, gin.H{
			"message": "Data saved successfully",
		})
	})

	r.GET("/read", func(c *gin.Context) {
		key := c.Query("key")
		value := database.Read([]byte(key))
		c.JSON(http.StatusOK, gin.H{
			"value": string(value),
		})
	})

	r.Static("/public", "./public")
	// Start the server
	err := r.Run(":3000")
	if err != nil {
		fmt.Println(err)
	}
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
