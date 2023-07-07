package hive

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/btcsuite/btcutil"
	"io/ioutil"
	"log"
	"net/http"
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

func main() {
	// Fetch the current block data
	resp, err := http.Get("https://api.hive.blog/get_dynamic_global_properties")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	var blockData BlockData
	err = json.Unmarshal(body, &blockData)
	if err != nil {
		log.Fatal(err)
	}
	previousBlock, err := hex.DecodeString(blockData.Previous[0:8])
	if err != nil {
		log.Fatal(err)
	}
	refBlockNum := binary.BigEndian.Uint16(previousBlock[0:2])
	refBlockPrefix := binary.LittleEndian.Uint32(previousBlock[2:6])
	expiration := blockData.Timestamp.Add(60 * time.Second)

	// Create the transfer operation
	transferOp := TransferOperation{
		From:   "nathansenn",
		To:     "dbuzz",
		Amount: "0.001 HIVE",
		Memo:   "test",
	}
	transferOpBytes, err := json.Marshal(transferOp)
	if err != nil {
		log.Fatal(err)
	}

	// Create the transaction
	operations := make([][]byte, 0)
	operations = append(operations, transferOpBytes)
	extensions := make([][]byte, 0)

	// Serialize the transaction
	var buffer bytes.Buffer
	binary.Write(&buffer, binary.LittleEndian, refBlockNum)
	binary.Write(&buffer, binary.LittleEndian, refBlockPrefix)
	binary.Write(&buffer, binary.LittleEndian, expiration.Unix())
	binary.Write(&buffer, binary.LittleEndian, uint16(len(operations)))
	buffer.Write(operations[0])
	binary.Write(&buffer, binary.LittleEndian, uint16(len(extensions)))

	// Create the digest
	chainID, _ := hex.DecodeString("beeab0de00000000000000000000000000000000000000000000000000000000")
	input := append(chainID, buffer.Bytes()...)
	digest := sha256.Sum256(input)

	// Sign the transaction
	privKey := "5K6yGu6gugkEumRDbwN4K7GStbizPYym4gVH5ywTZqpCECNhc58"
	wif, err := btcutil.DecodeWIF(privKey)
	if err != nil {
		log.Fatal(err)
	}
	privateKey := wif.PrivKey
	sig, err := privateKey.Sign(digest[:])
	if err != nil {
		log.Fatal(err)
	}

	// Broadcast the transaction
	signedTransaction := map[string]interface{}{
		"ref_block_num":    refBlockNum,
		"ref_block_prefix": refBlockPrefix,
		"expiration":       expiration.Format("2006-01-02T15:04:05"),
		"operations":       [][]interface{}{{"transfer", transferOp}},
		"extensions":       []string{},
		"signatures":       []string{hex.EncodeToString(sig.Serialize())},
	}
	jsonData, err := json.Marshal(signedTransaction)
	if err != nil {
		log.Fatal(err)
	}
	resp, err = http.Post("https://api.hive.blog", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(body))
}
