package validation

import (
	"encoding/json"
	"proofofaccess/database"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/proofcrypto"

	log "github.com/sirupsen/logrus"
)

const Layout = "2006-01-02 15:04:05.999999 -0700 MST m=+0.000000000"

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
	fileContents := file.String()
	//fmt.Println("hash: ", hash)
	fileContents += hash
	return fileContents
}

// CreatProofHash
// Create Proof Hash
func CreatProofHash(hash string, CID string) string {
	// Get all the file blocks CIDs from the Target Files CID
	log.Debug("Proof CID: ", CID)

	var cids []string
	var err error

	// Try to get refs from database first (for storage nodes)
	if database.DB != nil {
		refsBytes := database.Read([]byte("refs" + CID))
		if refsBytes != nil {
			if err := json.Unmarshal(refsBytes, &cids); err != nil {
				log.Errorf("Error while unmarshaling refs from database: %v\n", err)
			}
		}
	}

	// If no refs found in database or database not available, get from IPFS
	if len(cids) == 0 {
		log.Debugf("Getting refs for CID %s from IPFS (database not available or refs not found)", CID)
		cids, err = ipfs.Refs(CID)
		if err != nil {
			log.Errorf("Error getting refs from IPFS for CID %s: %v", CID, err)
			return ""
		}
	}

	// Get the length of the CIDs
	length := len(cids)
	log.Debug("length", length)
	// Create the file contents
	proofHash := ""
	// Get the seed from the hash
	var seed = 0
	if length > 0 {
		seed = int(proofcrypto.GetIntFromHash(hash, uint32(length)))
	}
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
		}
	}
	// Create the proof hash
	proofHash = proofcrypto.HashFile(proofHash)
	log.Debug("Proof Hash: ", proofHash)
	return proofHash
}

// ProofRequestJson
// Validation Node Functions
func ProofRequestJson(hash string, CID string) ([]byte, error) {
	requestProof := RequestProof{
		Type:   "RequestProof",
		Hash:   hash,
		CID:    CID,
		Status: "Pending",
		User:   localdata.GetNodeName(),
	}
	requestProofJson, err := json.Marshal(requestProof)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return requestProofJson, nil
}

// SelectIPFSRefs
// Get all the file blocks CIDs from the Target Files CID
func SelectIPFSRefs(CID string, hash string) []string {
	cids, _ := ipfs.Refs(CID)
	return cids
}
