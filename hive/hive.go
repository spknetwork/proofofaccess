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
		if err != nil || len(history) == 0 {
			break
		}

		for _, resultSlice := range history {
			if len(resultSlice) < 2 {
				continue
			}

			transactionData, ok := resultSlice[1].(map[string]interface{})
			if !ok {
				continue
			}

			transactionBytes, _ := json.Marshal(transactionData)
			var transaction TransactionEntry
			err = json.Unmarshal(transactionBytes, &transaction)
			if err != nil {
				continue
			}

			if transaction.Op.Type == "transfer_operation" {
				transferData := transaction.Op.Value
				if transferData.From == accountToCheck && transferData.To != accountToCheck {
					amount, _ := strconv.ParseFloat(transferData.Amount.Amount, 64)
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
			}
		}

	}

	return sentAmounts
}
