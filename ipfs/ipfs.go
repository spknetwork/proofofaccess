package ipfs

import (
	"bytes"
	"fmt"
	ipfs "github.com/ipfs/go-ipfs-api"
	"io/ioutil"
)

// Shell
// Create a new IPFS shell
var Shell = ipfs.NewLocalShell()

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
	fmt.Println("Checking if CID is pinned")
	pins, err := Shell.Pins()
	if err != nil {
		fmt.Println("Error getting pins:", err)
		return false
	}

	_, ok := pins[cid]
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

func IpfsPeerID() {
	// Get the IPFS peer ID
	peerID, err := Shell.ID()
	if err != nil {
		fmt.Println("Error getting peer ID:", err)
		return
	}
	fmt.Println("Peer ID:", peerID.ID)
}
