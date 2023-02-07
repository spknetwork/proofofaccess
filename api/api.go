package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"proofofaccess/localdata"
	"proofofaccess/proofcrypto"
	"proofofaccess/pubsub"
	"proofofaccess/validation"
	"strconv"
	"time"
)

// Define a struct to represent the data we want to return from the API
type ExampleResponse struct {
	Status  string `json:"Status"`
	Elapsed string `json:"Elapsed"`
}

var CID = ""

func Api() {
	http.Handle("/", http.FileServer(http.Dir("public")))
	http.HandleFunc("/validate", func(w http.ResponseWriter, r *http.Request) {
		// Check that the request is a GET request
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Check that the request contains a "name" query parameter
		name := r.URL.Query().Get("name")
		pubsub.Subscribe(name)
		if name == "" {
			http.Error(w, "Please provide a name", http.StatusBadRequest)
			return
		}
		CID = r.URL.Query().Get("CID")
		localdata.SaveCID(CID)
		if CID == "" {
			http.Error(w, "Please provide a CID", http.StatusBadRequest)
			return
		}

		hash := proofcrypto.CreateRandomHash()
		proofJson, _ := validation.ProofRequestJson(hash, CID)
		fmt.Println("Sending message:", proofJson)
		localdata.SaveTime()
		pubsub.Publish(proofJson)
		for status := localdata.GetStatus(); status == "pending"; status = localdata.GetStatus() {
			time.Sleep(1 * time.Second)
			fmt.Println("Waiting for proof")
		}
		// Create an instance of the ExampleResponse struct
		status := localdata.GetStatus()
		localdata.SetStatus("pending")
		elapsed := localdata.GetElapsed()
		fmt.Printf("Time elapsed: %s\n", elapsed)
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
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println(err)
	}
}
