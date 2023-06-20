package ipfs

import (
	"bytes"
	"encoding/json"
	"fmt"
	ipfs "github.com/ipfs/go-ipfs-api"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"proofofaccess/localdata"
	"strings"
)

// Shell
// Create a new IPFS shell
// Get flag for IPFS port
var Shell *ipfs.Shell

var Pins = make(map[string]interface{})
var NewPins = make(map[string]interface{})

var AllPins = map[string]ipfs.PinInfo{}

// Download
// Add a file to IPFS
func Download(fileHash string) (bytes.Buffer, error) {
	// Download the file from IPFS
	fmt.Println("Downloading file: ", fileHash)
	fileReader, err := Shell.Cat(fileHash)
	if err != nil {
		return bytes.Buffer{}, fmt.Errorf("error downloading file: %v", err)
	}
	defer fileReader.Close()
	fileContents, err := ioutil.ReadAll(fileReader)
	if err != nil {
		return bytes.Buffer{}, fmt.Errorf("error reading file contents: %v", err)
	}
	return *bytes.NewBuffer(fileContents), nil
}

// Refs
// Get all the file blocks CIDs from the Target Files CID
func Refs(CID string) ([]string, error) {
	cids, err := Shell.Refs(CID, true)
	if err != nil {
		fmt.Println("Error getting refs:", err)
		return nil, err
	}

	var cidsList []string
	for cid := range cids {
		//fmt.Println(cid)
		cidsList = append(cidsList, cid)
	}

	return cidsList, nil
}

func IsPinned(cid string) bool {
	// Check if the CID is pinned
	_, ok := localdata.SavedRefs[cid]
	return ok
}

func IpfsPingNode(peerID string) error {
	// Ping the specified node using its peer ID
	peer, err := Shell.FindPeer(peerID)
	if err != nil {
		return fmt.Errorf("error pinging node: %v", err)
	}
	fmt.Println("Peer Addrs", peer.Addrs)

	return nil
}

func IpfsPeerID() string {
	// Get the IPFS peer ID
	peerID, err := Shell.ID()
	if err != nil {
		fmt.Println("Error getting peer ID:", err)
		return ""
	}
	fmt.Println("Peer ID:", peerID.ID)
	return peerID.ID
}

// GetIPFromPeerID takes a string representation of a peer ID and returns the public IP address of the peer if available.
func GetIPFromPeerID(peerIDStr string) (string, error) {
	fmt.Println("Getting IP from Peer ID: ", peerIDStr)
	peerInfo, err := Shell.FindPeer(peerIDStr)
	if err != nil {
		return "", fmt.Errorf("failed to find peer: %v", err)
	}

	if len(peerInfo.Addrs) == 0 {
		return "", fmt.Errorf("no addresses found for peer ID: %s", peerIDStr)
	}

	var publicIP string
	fmt.Println("Peer Addrs", peerInfo.Addrs)
	for _, addr := range peerInfo.Addrs {
		ipAddr := strings.Split(addr, "/")[2]
		fmt.Println("IP Address:", ipAddr)

		ip := net.ParseIP(ipAddr)
		if ip != nil && !isPrivateIP(ip) {
			publicIP = ipAddr
		}
	}

	if publicIP == "" {
		return "", fmt.Errorf("no public IP address found for peer ID: %s", peerIDStr)
	}
	fmt.Println("Public IP:", publicIP)
	return publicIP, nil
}

// isPrivateIP checks if the given IP address is private.
func isPrivateIP(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}

	private := [][]byte{
		[]byte{10, 0, 0, 0},
		[]byte{172, 16, 0, 0},
		[]byte{192, 168, 0, 0},
	}

	mask := [][]byte{
		[]byte{255, 0, 0, 0},
		[]byte{255, 240, 0, 0},
		[]byte{255, 255, 0, 0},
	}

	for i := range private {
		if (ip4[0]&mask[i][0] == private[i][0]) &&
			(ip4[1]&mask[i][1] == private[i][1]) &&
			(ip4[2]&mask[i][2] == private[i][2]) &&
			(ip4[3]&mask[i][3] == private[i][3]) {
			return true
		}
	}

	return false
}

// UploadTextToFile
// Write text to a local file and then upload it to IPFS
func UploadTextToFile(text, filePath string) (string, error) {
	// Write the given text to a file
	err := ioutil.WriteFile(filePath, []byte(text), 0644)
	if err != nil {
		return "", fmt.Errorf("error writing to file: %v", err)
	}

	// Read the file
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("error reading file: %v", err)
	}
	defer file.Close()

	// Upload the file to IPFS
	cid, err := Shell.Add(file)
	if err != nil {
		return "", fmt.Errorf("error uploading file to IPFS: %v", err)
	}

	// Optional: remove the local file after uploading
	err = os.Remove(filePath)
	if err != nil {
		fmt.Printf("warning: error removing local file: %v\n", err)
	}

	return cid, nil
}

// DownloadAndDecodeJSON downloads a JSON file from IPFS and decodes its contents.
// The result is stored in the given destination.
func DownloadAndDecodeJSON(fileHash string, dest interface{}) error {
	// Download the file from IPFS
	fmt.Println("Downloading file: ", fileHash)
	fileReader, err := Shell.Cat(fileHash)
	if err != nil {
		return fmt.Errorf("error downloading file: %v", err)
	}
	defer fileReader.Close()

	// Read the file into a bytes buffer
	fileContents, err := ioutil.ReadAll(fileReader)
	if err != nil {
		return fmt.Errorf("error reading file contents: %v", err)
	}

	// Decode the JSON contents of the file into the destination
	err = json.Unmarshal(fileContents, dest)
	if err != nil {
		return fmt.Errorf("error decoding JSON: %v", err)
	}

	return nil
}
func SyncNode(allPins map[string]ipfs.PinInfo, name string) {
	Pins = NewPins
	fmt.Println("Fetching pins...")
	AllPins = allPins
	fmt.Println("Number of pins: ", len(allPins))
	if len(allPins) == 0 {
		fmt.Println("No pins found")
		return
	}
	NewPins = make(map[string]interface{})
	for key, pinInfo := range allPins {
		if pinInfo.Type == "recursive" {
			NewPins[key] = key
		}
	}
	fmt.Println("Number of recursive pins: ", len(NewPins))
	// Calculate the length of the map and the number of keys not found in Pins
	mapLength := len(NewPins)
	keysNotFound := 0
	fmt.Println("Syncing...")
	// Iterate through the keys in NewPins
	for key := range NewPins {
		size, _ := FileSize(key)
		fmt.Println("Size: ", size)
		fmt.Println("Name: ", name)
		fmt.Println("PeerSize: ", localdata.PeerSize[name])
		localdata.PeerSize[name] = localdata.PeerSize[name] + size
		// Check if the key exists in Pins
		if _, exists := Pins[key]; !exists {
			// If the key doesn't exist in Pins, add it to the pinsNotIncluded map
			fmt.Println("Key not found: ", key)
			var err error
			localdata.SavedRefs[key], err = Refs(key)
			if err != nil {
				fmt.Println("Error: ", err)
			}
			println("SavedRefs: ", len(localdata.SavedRefs[key]))

		} else {
			fmt.Println("Key found: ", key)
		}
		fmt.Println("Key: ", key)
		// Print the progress of the sync
		fmt.Println("Keys: ", keysNotFound)
		fmt.Println("Synced", name, " : ", float64(keysNotFound)/float64(mapLength)*100, "%")
		keysNotFound++
		if keysNotFound == mapLength {
			fmt.Println("Synced: ", name)
			localdata.NodesStatus[name] = "Synced"
			return
		}
	}
	localdata.NodesStatus[name] = "Failed"
	return
}
func FileSize(cid string) (int, error) {
	// Stat the object from IPFS
	objectStats, err := Shell.ObjectStat(cid)
	if err != nil {
		return 0, fmt.Errorf("error retrieving object stats: %v", err)
	}

	// objectStats.DataSize is the size of the file in bytes.
	return objectStats.CumulativeSize, nil
}

// RunGC runs the garbage collector on the IPFS node.
func RunGC() error {
	fmt.Println("Running IPFS garbage collector...")

	// Replace with your IPFS API address if different
	apiURL := "http://localhost:5001/api/v0/repo/gc"

	// Create new request to `repo gc` endpoint
	resp, err := http.Get(apiURL)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Check the HTTP status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("error in response from IPFS node: %s", string(bodyBytes))
	}

	fmt.Println("Garbage collection complete.")
	return nil
}
