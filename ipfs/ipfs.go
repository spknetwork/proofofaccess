package ipfs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	ipfs "github.com/ipfs/go-ipfs-api"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"proofofaccess/database"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"strings"
	"sync"
	"time"
)

// Shell
// Create a new IPFS shell
// Get flag for IPFS port
var Shell *ipfs.Shell
var lock sync.Mutex
var Pins = make(map[string]interface{})
var NewPins = make(map[string]interface{})
var AllPins = map[string]ipfs.PinInfo{}

const BufferSize = 1024 // define a reasonable buffer size
// Download
// Download the file from IPFS
func Download(fileHash string) (*bytes.Buffer, error) {
	// Download the file from IPFS
	fileReader, err := Shell.Cat(fileHash)
	if err != nil {
		return nil, fmt.Errorf("error downloading file: %v", err)
	}
	defer fileReader.Close()

	var result bytes.Buffer
	buf := make([]byte, BufferSize)
	totalRead := 0

	for {
		n, err := fileReader.Read(buf)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("error reading file contents: %v", err)
		}
		if n == 0 {
			break
		}

		result.Write(buf[:n])
		totalRead += n
		//fmt.Printf("Downloaded %d of %d bytes (%.2f%%)\n",
		//totalRead, fileSize, 100*float64(totalRead)/float64(fileSize))
	}

	return &result, nil
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
func IsPinnedInDB(cid string) bool {
	//fmt.Println("Checking if CID is pinned in the database")
	// Check if the CID is pinned in the database
	val := database.Read([]byte("refs" + cid))
	if val != nil {
		//fmt.Println("CID is pinned in the database")
		return true
	} else {
		//fmt.Println("CID is not pinned in the database")
		return false
	}
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
func FetchPins(ctx context.Context) {
	newPins := false // Assuming this is a boolean based on your usage

	for {
		select {
		case <-ctx.Done():
			return
		default:
			localdata.Lock.Lock()
			Pins = NewPins
			var PeerSize = 0
			localdata.Lock.Unlock()

			fmt.Println("Fetching pins...")
			allPins, err := Shell.Pins()
			fmt.Println("Fetched pins")
			for _, cid := range messaging.PinFileCids {
				delete(allPins, cid)
			}
			AllPins = allPins
			// Fetch all pins
			if err != nil {
				fmt.Println("Error fetching pins:", err)
				continue
			}

			localdata.Lock.Lock()
			NewPins = make(map[string]interface{})
			localdata.Lock.Unlock()

			for key, pinInfo := range allPins {
				if pinInfo.Type == "recursive" {
					localdata.Lock.Lock()
					NewPins[key] = key
					localdata.Lock.Unlock()
				}
			}

			// Calculate the length of the map and the number of keys not found in Pins
			mapLength := len(NewPins)

			keysNotFound := 0

			// Create a WaitGroup to wait for the function to finish
			var wg sync.WaitGroup

			// Iterate through the keys in NewPins
			for key := range NewPins {
				wg.Add(1)
				go func(key string) {
					defer wg.Done()
					// Check if the key exists in Pins
					localdata.Lock.Lock()
					localdata.Lock.Unlock()
					size, _ := FileSize(key)
					localdata.Lock.Lock()
					PeerSize += size
					//fmt.Println("Peer size: ", localdata.PeerSize[localdata.NodeName])
					localdata.Lock.Unlock()
					if !IsPinnedInDB(key) {
						localdata.Lock.Lock()
						newPins = true
						localdata.Lock.Unlock()

						// If the key doesn't exist in Pins, add it to the pinsNotIncluded map
						savedRefs, _ := Refs(key)
						localdata.Lock.Lock()
						localdata.SavedRefs[key] = savedRefs
						refsBytes, err := json.Marshal(savedRefs)
						if err != nil {
							log.Printf("Error: %v\n", err)
							return
						}
						database.Save([]byte("refs"+key), refsBytes)
						localdata.Lock.Unlock()
						localdata.Lock.Lock()
						localdata.SyncedPercentage = float32(keysNotFound) / float32(mapLength) * 100
						fmt.Println("Synced: ", localdata.SyncedPercentage, "%")
						localdata.Lock.Unlock()
						keysNotFound++
					}
				}(key)
			}

			wg.Wait()

			fmt.Println("Synced: ", 100)
			localdata.Lock.Lock()
			localdata.PeerSize[localdata.NodeName] = PeerSize
			localdata.SyncedPercentage = 100
			localdata.Lock.Unlock()

			if newPins {
				fmt.Println("New pins found")
				messaging.SendCIDS("validator1")
				newPins = false
			}

			time.Sleep(60 * time.Second)
		}
	}
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
func SyncNode(NewPins map[string]interface{}, name string) {
	fmt.Println("Syncing node: ", name)
	if len(NewPins) == 0 {
		fmt.Println("No pins found")
		return
	}

	mapLength := len(NewPins)
	keyCount := 0

	// Iterate through the keys in NewPins
	var peersize = 0

	// Create a slice to hold each CID's ref count
	refCounts := make([]int, len(NewPins))

	// Create a slice to hold each CID's total size
	sizes := make([]int, len(NewPins))

	// Create a slice to hold the completion status of each CID
	completed := make([]bool, len(NewPins))

	// Create a WaitGroup to wait for the function to finish
	var wg sync.WaitGroup

	// Function to print the progress of each CID
	printProgress := func(i int, key string) {
		var percentage float64
		var percentageInt int
		if completed[i] {
			localdata.Lock.Lock()
			localdata.CIDRefPercentage[key] = 100
			localdata.CIDRefStatus[key] = true
			localdata.Lock.Unlock()
			percentage = 100.0
		} else if sizes[i] > 0 {
			percentage = float64(refCounts[i]*256*1024) / float64(sizes[i]) * 100
			percentageInt = int(percentage)
			localdata.Lock.Lock()
			cIDRefPercentage := localdata.CIDRefPercentage[key]
			localdata.Lock.Unlock()
			if percentageInt+1 > cIDRefPercentage {
				localdata.Lock.Lock()
				localdata.CIDRefPercentage[key] = percentageInt
				localdata.CIDRefStatus[key] = false
				localdata.Lock.Unlock()
			}

		}
		fmt.Printf("Name: %s, CID: %s has %d references so far (%.2f%%)\n", name, key, refCounts[i], percentage)
	}

	index := 0
	for key := range NewPins {
		wg.Add(1)
		go func(i int, key string) {
			fmt.Println("Starting goroutine for: ", key)
			defer wg.Done()
			size, _ := FileSize(key)
			localdata.Lock.Lock()
			peersize = peersize + size
			localdata.Lock.Unlock()
			// Check if the key exists in Pins
			fmt.Println("Checking if key exists: ", key)
			fmt.Println("name: ", name)
			isPinnedInDB := IsPinnedInDB(key)
			if isPinnedInDB == false {
				if localdata.NodesStatus[name] != "Synced" {
					localdata.Lock.Lock()
					localdata.PeerSize[name] = peersize
					localdata.Lock.Unlock()
				}
				fmt.Println("Key not found: ", key)
				// Get the size of the file
				stat, err := Shell.ObjectStat(key)
				if err != nil {
					fmt.Printf("Error getting size: %v\n", err)
					return
				}
				sizes[i] = stat.CumulativeSize
				//fmt.Println("Getting refs: ", key)
				refs, err := Shell.Refs(key, true)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					return
				}
				var refsSlice []string
				for ref := range refs {
					refsSlice = append(refsSlice, ref)
					refCounts[i]++
					printProgress(i, key)
				}
				localdata.Lock.Lock()
				localdata.SavedRefs[key] = refsSlice
				localdata.Lock.Unlock()
				refsBytes, err := json.Marshal(refsSlice)
				if err != nil {
					log.Printf("Error: %v\n", err)
					return
				}
				fmt.Println("Saving refs: ", key)
				database.Save([]byte("refs"+key), refsBytes)
				completed[i] = true
			} else {
				localdata.Lock.Lock()
				nodesStatus := localdata.NodesStatus[name]
				localdata.Lock.Unlock()
				if nodesStatus != "Synced" {
					localdata.Lock.Lock()
					localdata.PeerSize[name] = peersize
					localdata.Lock.Unlock()
				}
				fmt.Println("Key found: ", key)
			}
			localdata.Lock.Lock()
			localdata.CIDRefPercentage[key] = 100
			localdata.CIDRefStatus[key] = true
			localdata.Lock.Unlock()
			keyCount++
			fmt.Println("Key: ", keyCount)
			fmt.Println("Map length: ", mapLength)
			if keyCount == mapLength {
				//fmt.Println("All keys found")
				localdata.Lock.Lock()
				localdata.PeerSize[name] = peersize
				fmt.Println("Synced: ", name)
				fmt.Println("PeerSize: ", peersize)
				localdata.NodesStatus[name] = "Synced"
				localdata.Lock.Unlock()
				return
			}
		}(index, key)
		index++
	}
	wg.Wait() // Wait for all goroutines to finish
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
