package validation

import (
	"encoding/json"
	"fmt"
	"proofofaccess/ipfs"
	"proofofaccess/proofcrypto"
)

type RequestProof struct {
	Type   string `json:"type"`
	Hash   string `json:"hash"`
	CID    string `json:"CID"`
	Status string `json:"status"`
}

// AppendHashToFile
// Download CID from IPFS and Append Hash to File contents
func AppendHashToFile(hash string, CID string) string {
	file, _ := ipfs.Download(CID)
	fileContents := file.String()
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
	// Create the file contents
	file := ""
	// Get the seed from the hash
	seed := int(proofcrypto.GetIntFromHash(hash))

	// Loop through all the CIDs and append the hash to the file contents
	for i := 1; i <= length; i++ {
		// If the seed is greater than the length of the CIDs, break
		if seed >= length {
			break
		}
		// If the seed is equal to the current index, append the hash to the file contents
		if i == seed {
			fmt.Println("Seed: ", seed)
			fmt.Println("Length ", length)
			file = file + AppendHashToFile(hash, cids[seed])
			seed = seed + int(proofcrypto.GetIntFromHash(hash+string(i)))

		}
	}
	// Create the proof hash
	proofHash := proofcrypto.HashFile(file)
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
