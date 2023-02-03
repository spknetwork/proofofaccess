package validation

import (
	"encoding/json"
	"fmt"
	"proofofaccess/ipfs"
	"proofofaccess/proofcrypto"
)

func AppendHashToFile(hash string, CID string) string {
	file, _ := ipfs.Download(CID)
	fileContents := file.String()
	fileContents += hash
	return fileContents
}

// Access Node Function
func CreatProofHash(hash string, CID string) string {
	file := AppendHashToFile(hash, CID)
	proofhash := proofcrypto.HashFile(file)
	//fmt.Println("Proof Hash: ", proofhash)
	return proofhash
}

type RequestProof struct {
	Type string `json:"type"`
	Hash string `json:"hash"`
	CID  string `json:"CID"`
}

// Validation Node Functions
func ProofRequestJson(hash string, CID string) (string, error) {
	requestProof := RequestProof{
		Type: "RequestProof",
		Hash: hash,
		CID:  CID,
	}
	requestProofJson, err := json.Marshal(requestProof)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return string(requestProofJson), nil
}

func ValidateProofHash(hash string, CID string, validationHash string) bool {
	file := AppendHashToFile(hash, CID)
	proofhash := proofcrypto.HashFile(file)
	if proofhash == validationHash {
		return true
	}
	return false
}
