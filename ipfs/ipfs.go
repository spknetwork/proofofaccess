package ipfs

import (
	"bytes"
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
	"strings"
	"sync"
	"time"
)

var Shell *ipfs.Shell
var AllPins = map[string]ipfs.PinInfo{}

const BufferSize = 1024

var isIPFSDown bool
var lastIPFSErrorTime time.Time

const ipfsErrorCooldown = 5 * time.Minute

func checkIPFSConnection() bool {
	if time.Since(lastIPFSErrorTime) < ipfsErrorCooldown {
		return false
	}

	_, err := Shell.ID()
	if err != nil {
		if !isIPFSDown {
			log.Println("IPFS node is down:", err)
			isIPFSDown = true
			lastIPFSErrorTime = time.Now()
		}
		return false
	}

	if isIPFSDown {
		log.Println("IPFS node is back online")
		isIPFSDown = false
	}
	return true
}

func Download(fileHash string) (*bytes.Buffer, error) {
	if !checkIPFSConnection() {
		return nil, fmt.Errorf("IPFS node is currently unavailable")
	}

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
	}

	return &result, nil
}

func Refs(CID string) ([]string, error) {
	if !checkIPFSConnection() {
		return nil, fmt.Errorf("IPFS node is currently unavailable")
	}

	cids, err := Shell.Refs(CID, true)
	if err != nil {
		return nil, fmt.Errorf("error getting refs: %v", err)
	}

	var cidsList []string
	for cid := range cids {
		cidsList = append(cidsList, cid)
	}

	return cidsList, nil
}

func IsPinned(cid string) bool {
	if !checkIPFSConnection() {
		log.Println("Cannot check if CID is pinned: IPFS is currently unavailable")
		return false
	}

	_, ok := localdata.SavedRefs[cid]
	return ok
}

func IsPinnedInDB(cid string) bool {
	val := database.Read([]byte("refs" + cid))
	return val != nil
}

func IpfsPingNode(peerID string) error {
	if !checkIPFSConnection() {
		return fmt.Errorf("IPFS node is currently unavailable")
	}

	peer, err := Shell.FindPeer(peerID)
	if err != nil {
		return fmt.Errorf("error pinging node: %v", err)
	}
	fmt.Println("Peer Addrs", peer.Addrs)

	return nil
}

func IpfsPeerID() string {
	if !checkIPFSConnection() {
		return ""
	}

	peerID, err := Shell.ID()
	if err != nil {
		log.Println("Error getting peer ID:", err)
		return ""
	}
	fmt.Println("Peer ID:", peerID.ID)
	return peerID.ID
}

func GetIPFromPeerID(peerIDStr string) (string, error) {
	if !checkIPFSConnection() {
		return "", fmt.Errorf("IPFS node is currently unavailable")
	}

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

func isPrivateIP(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}

	private := [][]byte{
		{10, 0, 0, 0},
		{172, 16, 0, 0},
		{192, 168, 0, 0},
	}

	mask := [][]byte{
		{255, 0, 0, 0},
		{255, 240, 0, 0},
		{255, 255, 0, 0},
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

func UploadTextToFile(text, filePath string) (string, error) {
	if !checkIPFSConnection() {
		return "", fmt.Errorf("IPFS node is currently unavailable")
	}

	err := ioutil.WriteFile(filePath, []byte(text), 0644)
	if err != nil {
		return "", fmt.Errorf("error writing to file: %v", err)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("error reading file: %v", err)
	}
	defer file.Close()

	cid, err := Shell.Add(file)
	if err != nil {
		return "", fmt.Errorf("error uploading file to IPFS: %v", err)
	}

	err = os.Remove(filePath)
	if err != nil {
		fmt.Printf("warning: error removing local file: %v\n", err)
	}

	return cid, nil
}

func DownloadAndDecodeJSON(fileHash string, dest interface{}) error {
	if !checkIPFSConnection() {
		return fmt.Errorf("IPFS node is currently unavailable")
	}

	fileReader, err := Shell.Cat(fileHash)
	if err != nil {
		return fmt.Errorf("error downloading file: %v", err)
	}
	defer fileReader.Close()

	fileContents, err := ioutil.ReadAll(fileReader)
	if err != nil {
		return fmt.Errorf("error reading file contents: %v", err)
	}

	err = json.Unmarshal(fileContents, dest)
	if err != nil {
		return fmt.Errorf("error decoding JSON: %v", err)
	}

	return nil
}

func SyncNode(NewPins map[string]interface{}, name string) {
	if !checkIPFSConnection() {
		log.Println("Cannot sync node: IPFS is currently unavailable")
		return
	}

	fmt.Println("Syncing node: ", name)
	if len(NewPins) == 0 {
		fmt.Println("No pins found")
		return
	}

	mapLength := len(NewPins)
	keyCount := 0
	var peersize = 0
	refCounts := make([]int, len(NewPins))
	sizes := make([]int, len(NewPins))
	completed := make([]bool, len(NewPins))
	var wg sync.WaitGroup

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
	}

	index := 0
	for key := range NewPins {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			size, _ := FileSize(key)
			localdata.Lock.Lock()
			peersize = peersize + size
			localdata.Lock.Unlock()

			isPinnedInDB := IsPinnedInDB(key)
			if !isPinnedInDB {
				if localdata.NodesStatus[name] != "Synced" {
					localdata.Lock.Lock()
					localdata.PeerSize[name] = peersize
					localdata.Lock.Unlock()
				}

				stat, err := Shell.ObjectStat(key)
				if err != nil {
					fmt.Printf("Error getting size: %v\n", err)
					return
				}
				sizes[i] = stat.CumulativeSize

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
			}
			localdata.Lock.Lock()
			localdata.CIDRefPercentage[key] = 100
			localdata.CIDRefStatus[key] = true
			localdata.Lock.Unlock()
			keyCount++
			if keyCount == mapLength {
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
	wg.Wait()
}

func FileSize(cid string) (int, error) {
	if !checkIPFSConnection() {
		return 0, fmt.Errorf("IPFS node is currently unavailable")
	}

	objectStats, err := Shell.ObjectStat(cid)
	if err != nil {
		return 0, fmt.Errorf("error retrieving object stats: %v", err)
	}

	return objectStats.CumulativeSize, nil
}

func SaveRefs(cids []string) {
	if !checkIPFSConnection() {
		log.Println("Cannot save refs: IPFS is currently unavailable")
		return
	}

	cidsMap := make(map[string]interface{})
	for _, cid := range cids {
		cidsMap[cid] = nil
	}
	var wg sync.WaitGroup
	refCounts := make([]int, len(cidsMap))
	completed := make([]bool, len(cidsMap))
	sizes := make([]int, len(cidsMap))

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
		fmt.Printf("CID: %s has %d references so far (%.2f%%)\n", key, refCounts[i], percentage)
	}

	index := 0
	for key := range cidsMap {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			isPinnedInDB := IsPinnedInDB(key)
			if !isPinnedInDB {
				stat, err := Shell.ObjectStat(key)
				if err != nil {
					fmt.Printf("Error getting size: %v\n", err)
					return
				}
				localdata.Lock.Lock()
				localdata.CidSize[key] = stat.CumulativeSize
				localdata.Lock.Unlock()
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
				database.Save([]byte("refs"+key), refsBytes)
				completed[i] = true
			}
			localdata.Lock.Lock()
			localdata.CIDRefPercentage[key] = 100
			localdata.CIDRefStatus[key] = true
			localdata.Lock.Unlock()
		}(index, key)
		index++
	}
	wg.Wait()
}
