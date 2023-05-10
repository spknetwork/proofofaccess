package hive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

var Endpoint = "https://api.hive.blog"

func GetIpfsID(user string) (string, error) {
	// Set the JSON-RPC request parameters
	payload := []byte(`{"id":25,"jsonrpc":"2.0","method":"condenser_api.get_accounts","params":[["` + user + `"]]} `)

	// Create a new HTTP request with the payload
	req, err := http.NewRequest("POST", Endpoint, bytes.NewBuffer(payload))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return "", err
	}

	// Set the headers for the request
	req.Header.Set("Content-Type", "application/json")

	// Send the request to the Hive API
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return "", err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return "", err
	}

	// Unmarshal the response JSON into a map[string]interface{}
	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		fmt.Println("Error unmarshalling response:", err)
		return "", err
	}

	// Extract the posting JSON metadata as a JSON object
	var result map[string]interface{}
	if data["result"].([]interface{})[0].(map[string]interface{}) != nil {
		result = data["result"].([]interface{})[0].(map[string]interface{})
	}

	postingMetadataStr := result["posting_json_metadata"].(string)
	var postingMetadata map[string]interface{}
	err = json.Unmarshal([]byte(postingMetadataStr), &postingMetadata)
	if err != nil {
		fmt.Println("Error unmarshalling posting JSON metadata:", err)
		return "", err
	}

	// Check if the ipfs_node_id field is present in the metadata
	ipfsNodeID, ok := postingMetadata["peerId"].(string)
	if !ok {
		fmt.Println("Error: profile field not found in posting JSON metadata")
		return "", err
	}

	// Print the IPFS node ID to the console and return it
	fmt.Println("IPFS node ID for", user, ":", ipfsNodeID)
	return ipfsNodeID, nil
}
