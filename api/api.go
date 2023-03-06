package api

import (
	"encoding/json"
	"fmt"
	"proofofaccess/database"
	"proofofaccess/localdata"
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

		// Pubsub channel
		pubsub.Subscribe(name)

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

		// Wait for the proof to be validated
		for status := localdata.GetStatus(hash); status == "Pending"; status = localdata.GetStatus(hash) {
			time.Sleep(30 * time.Millisecond)
		}

		// Get the proof status and time elapsed
		status := localdata.GetStatus(hash)
		elapsed := localdata.GetElapsed(hash)

		// Create a response struct
		response := ExampleResponse{Status: status, Elapsed: strconv.FormatFloat(float64(elapsed.Milliseconds()), 'f', 0, 64) + "ms"}

		// Encode the response as JSON
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			fmt.Println(err)
			return
		}

		// Write the JSON response to the WebSocket connection
		if err := conn.WriteMessage(websocket.TextMessage, jsonResponse); err != nil {
			fmt.Println(err)
			return
		}
	})

	r.Static("/public", "./public")
	// Start the server
	err := r.Run(":3000")
	if err != nil {
		fmt.Println(err)
	}
}
