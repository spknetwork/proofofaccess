package ipfs

import (
	"bytes"
	"fmt"
	ipfs "github.com/ipfs/go-ipfs-api"
	"io/ioutil"
	"net"
	"strings"
)

// Shell
// Create a new IPFS shell
var Shell = ipfs.NewShell("localhost:5001")
var Pins = map[string]ipfs.PinInfo{}

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
	_, ok := Pins[cid]
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
