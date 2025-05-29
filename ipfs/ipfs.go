package ipfs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"proofofaccess/database"
	"proofofaccess/localdata"
	"strings"
	"sync"
	"time"

	ipfs "github.com/ipfs/go-ipfs-api"
	multiaddr "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
)

var Shell *ipfs.Shell
var AllPins = map[string]ipfs.PinInfo{}

const BufferSize = 1024

var isIPFSDown bool
var lastIPFSErrorTime time.Time

const ipfsErrorCooldown = 30 * time.Second

func checkIPFSConnection() bool {
	if time.Since(lastIPFSErrorTime) < ipfsErrorCooldown {
		return false
	}

	_, err := Shell.ID()
	if err != nil {
		if !isIPFSDown {
			logrus.Errorf("IPFS node connection error: %v", err)
			isIPFSDown = true
			lastIPFSErrorTime = time.Now()
		}
		return false
	}

	if isIPFSDown {
		logrus.Info("IPFS node is back online")
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

	logrus.Debugf("Calling IPFS Refs command for CID: %s", CID)
	cids, err := Shell.Refs(CID, true)
	if err != nil {
		logrus.Errorf("Error in IPFS Refs command for CID %s: %v", CID, err)
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
		logrus.Warn("Cannot check if CID is pinned: IPFS is currently unavailable")
		return false
	}

	logrus.Debugf("Checking if CID %s is pinned", cid)
	localdata.Lock.Lock()
	_, ok := localdata.SavedRefs[cid]
	localdata.Lock.Unlock()
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

	logrus.Debugf("Calling IPFS swarm connect command for peer %s", peerID)
	// Use a more reliable method to check peer connectivity
	// The previous approach using routing/findpeer causes "nil Run function" errors
	resp, err := Shell.Request("swarm", "connect", "/p2p/"+peerID).Send(context.Background())
	if err != nil {
		logrus.Errorf("Error in IPFS swarm connect command for peer %s: %v", peerID, err)
		return fmt.Errorf("error connecting to peer %s via swarm connect: %w", peerID, err)
	}
	defer resp.Close()

	if resp.Error != nil {
		logrus.Errorf("IPFS swarm connect command failed for peer %s: %v", peerID, resp.Error)
		return fmt.Errorf("swarm connect command failed for %s: %w", peerID, resp.Error)
	}

	logrus.Debugf("Successfully connected to peer %s via swarm connect", peerID)
	return nil
}

func IpfsPeerID() string {
	if !checkIPFSConnection() {
		return ""
	}

	peerID, err := Shell.ID()
	if err != nil {
		logrus.Errorf("Error getting local IPFS Peer ID: %v", err)
		return ""
	}
	logrus.Infof("Local IPFS Peer ID: %s", peerID.ID)
	return peerID.ID
}

func GetIPFromPeerID(peerIDStr string) (string, error) {
	if !checkIPFSConnection() {
		return "", fmt.Errorf("IPFS node is currently unavailable")
	}

	logrus.Debugf("Calling IPFS swarm peers command to find IP for peer %s", peerIDStr)
	// Use swarm peers command instead of routing findpeer to avoid "nil Run function" errors
	res, err := Shell.Request("swarm", "peers").Send(context.Background())
	if err != nil {
		logrus.Errorf("Error in IPFS swarm peers command: %v", err)
		return "", fmt.Errorf("failed to get swarm peers: %w", err)
	}
	defer res.Close()

	if res.Error != nil {
		logrus.Errorf("IPFS swarm peers command failed: %v", res.Error)
		return "", fmt.Errorf("swarm peers command failed: %w", res.Error)
	}

	outputBytes, err := ioutil.ReadAll(res.Output)
	if err != nil {
		return "", fmt.Errorf("failed to read swarm peers response: %w", err)
	}

	var addresses []multiaddr.Multiaddr
	scanner := bufio.NewScanner(bytes.NewReader(outputBytes))
	for scanner.Scan() {
		line := scanner.Text()
		// Only process addresses that contain the peer ID we're looking for
		if strings.Contains(line, peerIDStr) {
			addr, err := multiaddr.NewMultiaddr(line)
			if err != nil {
				logrus.Warnf("Could not parse address '%s' from swarm peers output: %v", line, err)
				continue
			}
			addresses = append(addresses, addr)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error scanning swarm peers response: %w", err)
	}

	if len(addresses) == 0 {
		return "", fmt.Errorf("peer ID %s not found in swarm peers", peerIDStr)
	}

	var publicIP string
	for _, addr := range addresses {
		addrStr := addr.String()
		parts := strings.Split(addrStr, "/")
		if len(parts) < 3 {
			continue
		}
		ipAddr := parts[2]

		ip := net.ParseIP(ipAddr)
		if ip != nil && !isPrivateIP(ip) {
			publicIP = ipAddr
			break
		}
	}

	if publicIP == "" {
		// If no public IP was found in the direct connections,
		// we can't determine the IP address from the peer ID
		return "", fmt.Errorf("no public IP address found for peer ID: %s", peerIDStr)
	}
	logrus.Debugf("Found public IP %s for Peer ID %s", publicIP, peerIDStr)
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
		return "", fmt.Errorf("error writing to file %s: %v", filePath, err)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("error reading file %s: %v", filePath, err)
	}
	defer file.Close()

	cid, err := Shell.Add(file)
	if err != nil {
		return "", fmt.Errorf("error uploading file %s to IPFS: %v", filePath, err)
	}

	err = os.Remove(filePath)
	if err != nil {
		logrus.Warnf("Error removing local file %s after IPFS upload: %v", filePath, err)
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
	if len(NewPins) == 0 {
		logrus.Warnf("Attempted to sync node %s, but no pins provided.", name)
		return
	}
	localdata.Lock.Lock()
	localdata.PeerCids[name] = make([]string, 0, len(NewPins))
	localdata.Lock.Unlock()
	var totalSize int
	var wg sync.WaitGroup
	percentage := 0

	keys := make([]string, 0, len(NewPins))
	for k := range NewPins {
		keys = append(keys, k)
	}

	localdata.Lock.Lock()
	localdata.PeerSyncSeed[name] = 0
	localdata.CIDRefStatus[name] = false
	localdata.CIDRefPercentage[name] = percentage
	localdata.Lock.Unlock()

	// Add rate limiting for sync operations
	semaphore := make(chan struct{}, 3) // Limit to 3 concurrent operations for sync

	for i, key := range keys {
		wg.Add(1)
		go func(key string, index int) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			size, err := FileSize(key)
			if err != nil {
				logrus.Warnf("Error getting size for CID %s during sync with %s: %v", key, name, err)
				return
			}

			localdata.Lock.Lock()
			totalSize += size
			localdata.PeerCids[name] = append(localdata.PeerCids[name], key)
			localdata.CidSize[key] = size
			percentage = (index + 1) * 100 / len(keys)
			localdata.CIDRefPercentage[name] = percentage
			localdata.Lock.Unlock()

			if !IsPinnedInDB(key) {
				savedRefs, err := Refs(key)
				if err != nil {
					logrus.Debugf("Skipping refs for CID %s during sync with %s (may not exist): %v", key, name, err)
					return
				}
				localdata.Lock.Lock()
				localdata.SavedRefs[key] = savedRefs
				refsBytes, err := json.Marshal(savedRefs)
				if err != nil {
					logrus.Errorf("Error marshaling refs for CID %s: %v", key, err)
					localdata.Lock.Unlock()
					return
				}
				database.Save([]byte("refs"+key), refsBytes)
				localdata.Lock.Unlock()
			}
		}(key, i)
	}

	wg.Wait()

	localdata.Lock.Lock()
	localdata.CIDRefStatus[name] = true
	peersize := totalSize
	localdata.PeerSize[name] = peersize
	localdata.SyncedPercentage = 100
	localdata.Lock.Unlock()
	logrus.Debugf("Finished syncing with node %s. Total size: %d", name, peersize)
}

func FileSize(cid string) (int, error) {
	if !checkIPFSConnection() {
		return 0, fmt.Errorf("IPFS node is currently unavailable")
	}
	stat, err := Shell.ObjectStat(cid)
	if err != nil {
		return 0, fmt.Errorf("error getting stats for CID %s: %v", cid, err)
	}
	return stat.CumulativeSize, nil
}

func SaveRefs(cids []string) {
	logrus.Debugf("Saving refs for %d CIDs...", len(cids))
	var wg sync.WaitGroup
	percentage := 0

	// Add rate limiting to prevent overwhelming IPFS
	semaphore := make(chan struct{}, 5) // Limit to 5 concurrent operations

	for i, key := range cids {
		wg.Add(1)
		go func(key string, index int) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if !IsPinnedInDB(key) {
				savedRefs, err := Refs(key)
				if err != nil {
					logrus.Debugf("Skipping refs for CID %s in SaveRefs (may not exist): %v", key, err)
					return
				}
				localdata.Lock.Lock()
				localdata.SavedRefs[key] = savedRefs
				refsBytes, err := json.Marshal(savedRefs)
				if err != nil {
					logrus.Errorf("Error marshaling refs for CID %s in SaveRefs: %v", key, err)
					localdata.Lock.Unlock()
					return
				}
				database.Save([]byte("refs"+key), refsBytes)
				percentage = (index + 1) * 100 / len(cids)
				localdata.CIDRefPercentage["self"] = percentage
				localdata.Lock.Unlock()
			}
		}(key, i)
	}
	wg.Wait()
	logrus.Debug("Finished saving refs.")
}
