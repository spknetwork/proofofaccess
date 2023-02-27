package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"proofofaccess/database"
	"proofofaccess/localdata"
	"proofofaccess/proofcrypto"
	"proofofaccess/pubsub"
	"proofofaccess/validation"
	"strconv"
	"time"
)

// ExampleResponse
// Define a struct to represent the data we want to return from the API
type ExampleResponse struct {
	Status  string `json:"Status"`
	Elapsed string `json:"Elapsed"`
}

var CID = ""

// Api
// Starts the API and handles the requests
func Api() {
	// Handle the API requests
	http.Handle("/", http.FileServer(http.Dir("public")))

	http.HandleFunc("/validate", func(w http.ResponseWriter, r *http.Request) {
		// Check that the request is a GET request
		CheckRequestMethod(r, w, "GET")

		// Check that the request contains query parameters
		name := CheckRequestQuery(r, w, "name")
		CID = CheckRequestQuery(r, w, "cid")

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
		pubsub.Publish(proofJson, localdata.GetNodeName())

		// Wait for the proof to be validated
		for status := localdata.GetStatus(hash); status == "Pending"; status = localdata.GetStatus(hash) {
			time.Sleep(1 * time.Second)
		}

		// Get the proof status and time elapsed
		status := localdata.GetStatus(hash)
		elapsed := localdata.GetElapsed(hash)

		// Create a response struct
		response := ExampleResponse{Status: status, Elapsed: strconv.FormatFloat(float64(elapsed.Milliseconds()), 'f', 0, 64) + "ms"}

		// Encode the response as JSON
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write the JSON response to the response writer
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse)
	})

	// Start the server
	err := http.ListenAndServe(":3000", nil)
	if err != nil {
		fmt.Println(err)
	}
}

// CheckRequestQuery
// Check that the request contains a query parameter
func CheckRequestQuery(r *http.Request, w http.ResponseWriter, query string) string {
	name := r.URL.Query().Get(query)
	pubsub.Subscribe(name)
	if name == "" {
		http.Error(w, "Please provide a "+query, http.StatusBadRequest)
		return ""
	}
	return name
}

// CheckRequestMethod
// Check that the request is a GET request
func CheckRequestMethod(r *http.Request, w http.ResponseWriter, method string) {
	if r.Method != method {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}
}
