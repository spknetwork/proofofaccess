package ipfs

import (
	"bytes"
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
	"github.com/sirupsen/logrus"
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
		logrus.Warn("Cannot check if CID is pinned: IPFS is currently unavailable")
		return false
	}

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

	peer, err := Shell.FindPeer(peerID)
	if err != nil {
		return fmt.Errorf("error finding peer %s: %v", peerID, err)
	}
	logrus.Debugf("Found peer %s addresses: %v", peerID, peer.Addrs)

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

	peerInfo, err := Shell.FindPeer(peerIDStr)
	if err != nil {
		return "", fmt.Errorf("failed to find peer %s: %v", peerIDStr, err)
	}

	if len(peerInfo.Addrs) == 0 {
		return "", fmt.Errorf("no addresses found for peer ID: %s", peerIDStr)
	}

	var publicIP string
	for _, addr := range peerInfo.Addrs {
		ipAddr := strings.Split(addr, "/")[2]

		ip := net.ParseIP(ipAddr)
		if ip != nil && !isPrivateIP(ip) {
			publicIP = ipAddr
			break
		}
	}

	if publicIP == "" {
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

	for i, key := range keys {
		wg.Add(1)
		go func(key string, index int) {
			defer wg.Done()

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
					logrus.Warnf("Error getting refs for CID %s during sync with %s: %v", key, name, err)
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
	localdata.Lock.Unlock()

	logrus.Infof("Finished syncing with node %s. Total size: %d", name, peersize)
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
	logrus.Infof("Saving refs for %d CIDs...", len(cids))
	var wg sync.WaitGroup
	percentage := 0

	for i, key := range cids {
		wg.Add(1)
		go func(key string, index int) {
			defer wg.Done()
			if !IsPinnedInDB(key) {
				savedRefs, err := Refs(key)
				if err != nil {
					logrus.Warnf("Error getting refs for CID %s in SaveRefs: %v", key, err)
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
	logrus.Info("Finished saving refs.")
}
