package hive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
)

var Endpoint = "https://api.hive.blog"

func GetIpfsID(user string) (string, error) {
	// Set the JSON-RPC request parameters
	payload := []byte(`{"id":25,"jsonrpc":"2.0","method":"condenser_api.get_accounts","params":[["` + user + `"]]} `)

	// Create a new HTTP request with the payload
	req, err := http.NewRequest("POST", Endpoint, bytes.NewBuffer(payload))
	if err != nil {
		log.Errorf("Error creating request: %v", err)
		return "", err
	}

	// Set the headers for the request
	req.Header.Set("Content-Type", "application/json")

	// Send the request to the Hive API
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("Error sending request: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Error reading response: %v", err)
		return "", err
	}

	// Unmarshal the response JSON into a map[string]interface{}
	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		log.Errorf("Error unmarshalling response: %v", err)
		return "", err
	}

	// Extract the posting JSON metadata as a JSON object
	var result map[string]interface{}
	resultsArray, ok := data["result"].([]interface{})
	if !ok || len(resultsArray) == 0 {
		log.Errorf("Error: 'result' field is not an array or is empty in response for user %s", user)
		return "", fmt.Errorf("invalid response format: 'result' field missing or empty")
	}
	result, ok = resultsArray[0].(map[string]interface{})
	if !ok || result == nil {
		log.Errorf("Error: first element in 'result' is not an object or is nil for user %s", user)
		return "", fmt.Errorf("invalid response format: first result item is not an object")
	}

	postingMetadataStr, ok := result["posting_json_metadata"].(string)
	if !ok || postingMetadataStr == "" {
		log.Warnf("User %s has no posting_json_metadata or it's empty", user)
		return "", fmt.Errorf("posting_json_metadata not found or empty for user %s", user)
	}
	var postingMetadata map[string]interface{}
	err = json.Unmarshal([]byte(postingMetadataStr), &postingMetadata)
	if err != nil {
		log.Errorf("Error unmarshalling posting JSON metadata for user %s: %v", user, err)
		return "", err
	}

	// Check if the ipfs_node_id field is present in the metadata
	ipfsNodeID, ok := postingMetadata["peerId"].(string)
	if !ok {
		log.Warnf("Error: 'peerId' field not found in posting JSON metadata for user %s", user)
		return "", fmt.Errorf("peerId field not found in posting JSON metadata for user %s", user)
	}

	// Print the IPFS node ID to the console and return it
	log.Infof("IPFS node ID for %s: %s", user, ipfsNodeID)
	return ipfsNodeID, nil
}

type BlockData struct {
	Previous  string    `json:"previous"`
	Timestamp time.Time `json:"timestamp"`
}

type TransferOperation struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Amount string `json:"amount"` // "0.001 HIVE" or "0.001 HBD"
	Memo   string `json:"memo"`
}

const rpcEndpoint = "https://api.hive.blog"
const accountToCheck = "dbuzz.spk"

type Transfer struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Amount struct {
		Amount    string `json:"amount"`
		Precision int    `json:"precision"`
	} `json:"amount"`
}

type OperationData struct {
	Type  string   `json:"type"`
	Value Transfer `json:"value"`
}

type TransactionEntry struct {
	TrxID string        `json:"trx_id"`
	Block int           `json:"block"`
	Op    OperationData `json:"op"`
}

type RPCResponse struct {
	Status string `json:"status"`
	Result struct {
		History [][]interface{} `json:"history"`
	} `json:"result"`
	ID int `json:"id"`
}

func fetchHistory(start int) ([][]interface{}, error) {
	data := `{"jsonrpc":"2.0", "method":"account_history_api.get_account_history", "params":{"account":"` + accountToCheck + `", "start":` + strconv.Itoa(start) + `, "limit":1000}, "id":1}`
	resp, err := http.Post(rpcEndpoint, "application/json", bytes.NewBuffer([]byte(data)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var rpcResponse RPCResponse
	err = json.Unmarshal(body, &rpcResponse)
	if err != nil {
		return nil, err
	}

	return rpcResponse.Result.History, nil
}

func GetHiveSent() map[string]float64 {
	sentAmounts := make(map[string]float64)
	startIndex := -1

	for {
		history, err := fetchHistory(startIndex)
		if err != nil {
			log.Errorf("Error fetching history starting at %d: %v", startIndex, err)
			break
		}
		if len(history) == 0 {
			log.Info("No more history entries found.")
			break
		}
		log.Debugf("Fetched %d history entries starting at index %d", len(history), startIndex)
		for _, resultSlice := range history {
			if len(resultSlice) < 2 {
				log.Warnf("Skipping history entry with unexpected length: %d", len(resultSlice))
				continue
			}

			transactionData, ok := resultSlice[1].(map[string]interface{})
			if !ok {
				log.Warn("Skipping history entry where second element is not a map")
				continue
			}

			transactionBytes, err := json.Marshal(transactionData)
			if err != nil {
				log.Warnf("Error marshaling transaction data: %v", err)
				continue
			}
			var transaction TransactionEntry
			err = json.Unmarshal(transactionBytes, &transaction)
			if err != nil {
				log.Warnf("Error unmarshaling transaction entry: %v", err)
				continue
			}

			if transaction.Op.Type == "transfer_operation" {
				transferData := transaction.Op.Value
				if transferData.From == accountToCheck && transferData.To != accountToCheck {
					log.Debugf("Processing transfer: To: %s, Amount: %s", transferData.To, transferData.Amount.Amount)
					amount, err := strconv.ParseFloat(transferData.Amount.Amount, 64)
					if err != nil {
						log.Warnf("Error parsing transfer amount '%s': %v", transferData.Amount.Amount, err)
						continue
					}
					amount /= math.Pow(10, float64(transferData.Amount.Precision))
					sentAmounts[transferData.To] += amount
				}
			}
		}

		if len(history) < 1000 {
			break
		}
		if len(history[0]) > 0 {
			if index, ok := history[0][0].(float64); ok {
				startIndex = int(index) - 1
				if startIndex < 0 {
					log.Warn("Calculated negative start index, stopping history fetch.")
					break
				}
			} else {
				log.Warn("Could not determine next start index from history entry.")
				break
			}
		} else {
			log.Warn("First history entry is empty, cannot determine next start index.")
			break
		}
	}
	log.Infof("Finished fetching hive sent history for %s.", accountToCheck)
	return sentAmounts
}
