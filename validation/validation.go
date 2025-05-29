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
	log.Infof("=== Starting proof hash generation ===")
	log.Infof("  CID: %s", CID)
	log.Infof("  Input hash: %s", hash)
	log.Infof("  Database available: %t", database.DB != nil)

	var cids []string
	var err error

	// Try to get refs from database first (for storage nodes)
	if database.DB != nil {
		log.Debugf("Attempting to get refs from database for CID %s", CID)
		refsBytes := database.Read([]byte("refs" + CID))
		if refsBytes != nil {
			log.Debugf("Found refs in database for CID %s, unmarshaling...", CID)
			if err := json.Unmarshal(refsBytes, &cids); err != nil {
				log.Errorf("Error while unmarshaling refs from database for CID %s: %v", CID, err)
			} else {
				log.Infof("Successfully loaded %d refs from database for CID %s", len(cids), CID)
			}
		} else {
			log.Debugf("No refs found in database for CID %s", CID)
		}
	} else {
		log.Debugf("Database not available, will try IPFS directly")
	}

	// If no refs found in database or database not available, get from IPFS
	if len(cids) == 0 {
		log.Infof("Getting refs for CID %s from IPFS (database not available or refs not found)", CID)
		cids, err = ipfs.Refs(CID)
		if err != nil {
			log.Errorf("Error getting refs from IPFS for CID %s: %v", CID, err)
			log.Errorf("=== Proof hash generation failed - no refs available ===")
			return ""
		}
		log.Infof("Successfully got %d refs from IPFS for CID %s", len(cids), CID)
	}

	// Get the length of the CIDs
	length := len(cids)
	log.Infof("Total refs count: %d", length)

	if length == 0 {
		log.Errorf("No refs found for CID %s - cannot generate proof hash", CID)
		log.Errorf("=== Proof hash generation failed - zero refs ===")
		return ""
	}

	// Create the file contents
	proofHash := ""
	// Get the seed from the hash
	var seed = 0
	if length > 0 {
		seed = int(proofcrypto.GetIntFromHash(hash, uint32(length)))
		log.Debugf("Generated seed: %d from hash for length %d", seed, length)
	}

	// Loop through all the CIDs and append the hash to the file contents
	log.Debugf("Starting proof hash calculation loop...")
	for i := 0; i <= length; i++ {
		// If the seed is greater than the length of the CIDs, break
		if seed >= length {
			log.Debugf("Seed %d >= length %d, breaking loop", seed, length)
			break
		}
		// If the seed is equal to the current index, append the hash to the file contents
		if i == seed {
			log.Debugf("Processing refs[%d] = %s", seed, cids[seed])
			fileHash := proofcrypto.HashFile(AppendHashToFile(hash, cids[seed]))
			proofHash = proofHash + fileHash
			log.Debugf("Added file hash: %s, current proofHash: %s", fileHash, proofHash)
			seed = seed + int(proofcrypto.GetIntFromHash(hash+proofHash, uint32(length)))
			log.Debugf("Updated seed to: %d", seed)
		}
	}

	// Create the proof hash
	finalHash := proofcrypto.HashFile(proofHash)
	log.Infof("=== Proof hash generation complete ===")
	log.Infof("  Final proof hash: %s", finalHash)
	log.Infof("  Input length: %d chars", len(proofHash))

	if finalHash == "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		log.Errorf("WARNING: Generated hash is empty string hash! Input was: '%s'", proofHash)
		log.Errorf("This indicates the proof calculation loop didn't add any content")
	}

	return finalHash
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
