package validation

import (
	"encoding/json"
	"fmt"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/proofcrypto"
)

type RequestProof struct {
	Type   string `json:"type"`
	Hash   string `json:"hash"`
	CID    string `json:"CID"`
	Status string `json:"status"`
	User   string `json:"user"`
}

// AppendHashToFile
// Download CID from IPFS and Append Hash to File contents
func AppendHashToFile(hash string, CID string) string {
	file, _ := ipfs.Download(CID)
	fmt.Println("File: ")
	fileContents := file.String()
	fmt.Println("hash: ", hash)
	fileContents += hash
	return fileContents
}

// CreatProofHash
// Create Proof Hash
func CreatProofHash(hash string, CID string) string {
	// Get all the file blocks CIDs from the Target Files CID
	cids := SelectIPFSRefs(CID, hash)
	// Get the length of the CIDs
	length := len(cids)
	fmt.Println("length", length)
	// Create the file contents
	proofHash := ""
	// Get the seed from the hash
	var seed = 0
	if length > 0 {
		seed = int(proofcrypto.GetIntFromHash(hash, uint32(length)))
	}
	fmt.Println("Seed: ", seed)
	// Loop through all the CIDs and append the hash to the file contents
	for i := 0; i <= length; i++ {
		// If the seed is greater than the length of the CIDs, break
		if seed >= length {
			break
		}
		// If the seed is equal to the current index, append the hash to the file contents
		if i == seed {
			proofHash = proofHash + proofcrypto.HashFile(AppendHashToFile(hash, cids[seed]))
			seed = seed + int(proofcrypto.GetIntFromHash(hash+proofHash, uint32(length)))
			fmt.Println("Seed: ", seed)
		}
	}
	// Create the proof hash
	proofHash = proofcrypto.HashFile(proofHash)
	fmt.Println("Proof Hash: ", proofHash)
	return proofHash
}

// ProofRequestJson
// Validation Node Functions
func ProofRequestJson(hash string, CID string) (string, error) {
	requestProof := RequestProof{
		Type:   "RequestProof",
		Hash:   hash,
		CID:    CID,
		Status: "Pending",
		User:   localdata.GetNodeName(),
	}
	requestProofJson, err := json.Marshal(requestProof)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return string(requestProofJson), nil
}

// SelectIPFSRefs
// Get all the file blocks CIDs from the Target Files CID
func SelectIPFSRefs(CID string, hash string) []string {
	cids, _ := ipfs.Refs(CID)
	return cids
}
