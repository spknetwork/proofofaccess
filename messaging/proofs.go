package messaging

import (
	"encoding/json"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/pubsub"
	"proofofaccess/validation"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// Structure to hold details from an individual proof response
type ProofResponse struct {
	Hash      string
	Elapsed   time.Duration
	Responder string
	Timestamp time.Time // When the response was received by the validator
}

// Map to store responses grouped by cid+seed (the key identifies the original request)
// Key: cid + seed
// Value: Slice of responses received so far
var pendingProofs = make(map[string][]ProofResponse)
var pendingProofsMutex sync.Mutex // Mutex to protect access to pendingProofs

// Map to track if consensus processing has been initiated for a key
// Exported variables
var ConsensusProcessing = make(map[string]bool)
var ConsensusProcessingMutex sync.Mutex

// HandleRequestProof
// This is the function that handles the request for proof from the validation node
func HandleRequestProof(req Request, ws *websocket.Conn) {
	CID := req.CID
	hash := req.Hash
	if ipfs.IsPinnedInDB(CID) == true {
		validationHash := validation.CreatProofHash(hash, CID)
		SendProof(req, validationHash, hash, localdata.NodeName, ws)
	} else {
		logrus.Warnf("Pin %s not found locally when handling proof request for %s", CID, req.User)
		SendProof(req, "NA", hash, localdata.NodeName, ws)
	}
}

// HandleProofOfAccess
// MODIFIED: Collects responses, validation happens later via consensus.
func HandleProofOfAccess(req Request, ws *websocket.Conn) {
	receivedTime := time.Now() // Record when the response arrived
	CID := req.CID
	seed := req.Seed

	if CID == "" || seed == "" {
		logrus.Errorf("Received ProofOfAccess message with missing CID (%s) or Seed (%s) from %s", CID, seed, req.User)
		return
	}

	key := CID + seed

	// Check if consensus already ran or is running for this key
	ConsensusProcessingMutex.Lock() // Use exported mutex
	processing, exists := ConsensusProcessing[key]
	ConsensusProcessingMutex.Unlock() // Use exported mutex
	if exists && processing {
		logrus.Debugf("Consensus already processing/processed for key %s. Discarding late response from %s.", key, req.User)
		return
	}

	// Get the start time to calculate elapsed time relative to the request
	start := localdata.GetTime(CID, seed) // Uses CID+seed key
	if start.IsZero() {
		// Can't calculate meaningful elapsed time relative to request.
		logrus.Warnf("Could not retrieve start time for CID %s, seed %s. Cannot process proof response from %s.", CID, seed, req.User)
		return
	}
	elapsed := receivedTime.Sub(start) // Elapsed = time response received - time request sent

	// Store the response temporarily
	response := ProofResponse{
		Hash:      req.Hash,
		Elapsed:   elapsed,
		Responder: req.User,
		Timestamp: receivedTime,
	}

	pendingProofsMutex.Lock()
	pendingProofs[key] = append(pendingProofs[key], response)
	logrus.Debugf("Collected proof response for key %s from %s. Elapsed: %v. Hash: %s. Total collected: %d", key, req.User, elapsed, req.Hash, len(pendingProofs[key]))
	pendingProofsMutex.Unlock()
}

// processProofConsensus is called after the timeout when waiting for proofs.
// Exported function.
func ProcessProofConsensus(cid string, seed string, targetName string, startTime time.Time) {
	key := cid + seed

	// Mark consensus as started to prevent late responses from being added
	ConsensusProcessingMutex.Lock() // Use exported mutex
	if processing, exists := ConsensusProcessing[key]; exists && processing {
		// Already processed or currently processing, avoid duplicate run
		ConsensusProcessingMutex.Unlock() // Use exported mutex
		logrus.Warnf("Consensus processing for key %s already initiated.", key)
		return
	}
	ConsensusProcessing[key] = true
	ConsensusProcessingMutex.Unlock() // Use exported mutex

	logrus.Infof("Processing proof consensus for key: %s", key)

	pendingProofsMutex.Lock()
	responses, exists := pendingProofs[key]
	if exists {
		// Clean up the entry from pending proofs map *after* processing
		defer func() {
			pendingProofsMutex.Lock()
			delete(pendingProofs, key)
			pendingProofsMutex.Unlock()
		}()
	}
	pendingProofsMutex.Unlock() // Unlock after accessing the slice

	if !exists || len(responses) == 0 {
		logrus.Warnf("No proof responses received for key %s within timeout.", key)
		localdata.SetStatus(seed, cid, "Invalid", "NoResponse", startTime, 0) // Use updated SetStatus
		return
	}

	// Filter timely responses and collect valid hashes
	var timelyResponses []ProofResponse
	var validHashes []string
	var firstValidHash string = ""
	conflictingHashes := false
	// Use a consistent timeout duration for checking timeliness
	validationTimeoutDuration := 25 * time.Second // Example: use the same threshold as before

	for _, resp := range responses {
		// Check timeliness (elapsed time from request to response reception)
		if resp.Elapsed < validationTimeoutDuration {
			timelyResponses = append(timelyResponses, resp)
			// Check hash validity and consistency within timely responses
			if resp.Hash != "NA" && resp.Hash != "" {
				validHashes = append(validHashes, resp.Hash)
				if firstValidHash == "" {
					firstValidHash = resp.Hash // Store the first valid hash encountered
				} else if firstValidHash != resp.Hash {
					conflictingHashes = true // Found a different valid hash
				}
			} else {
				logrus.Debugf("Timely response from %s for key %s had invalid hash: %s", resp.Responder, key, resp.Hash)
			}
		} else {
			logrus.Debugf("Response from %s for key %s discarded (too slow): %v", resp.Responder, key, resp.Elapsed)
		}
	}

	numTimelyResponses := len(timelyResponses)
	numValidHashes := len(validHashes) // Count of non-"NA", non-empty hashes within timely responses
	logrus.Infof("Consensus for key %s: %d total responses, %d timely, %d valid hashes.", key, len(responses), numTimelyResponses, numValidHashes)

	// Determine if we need to calculate the expected hash
	validationNeeded := false
	spotCheckHit := false
	if !startTime.IsZero() { // Check if startTime is valid before using
		spotCheckHit = startTime.Nanosecond()%1e6 == 0
	} else {
		logrus.Warnf("Cannot perform spot check for key %s: Original request start time is zero.", key)
	}

	if spotCheckHit {
		logrus.Infof("Spot check triggered for key %s (timestamp: %s)", key, startTime.String())
		validationNeeded = true
	} else if numValidHashes < 3 {
		logrus.Infof("Validation needed for key %s: Fewer than 3 valid hashes (%d received).", key, numValidHashes)
		validationNeeded = true
	} else if conflictingHashes {
		logrus.Infof("Validation needed for key %s: Conflicting valid hashes received.", key)
		validationNeeded = true
	} else if numValidHashes > 0 && !conflictingHashes && numValidHashes >= 3 {
		validationNeeded = false
		logrus.Infof("Validation not needed for key %s: >=3 timely, matching valid hashes and no spot check.", key)
	} else {
		logrus.Warnf("Defaulting to validation needed for key %s (numValidHashes: %d, conflicting: %t, spotCheck: %t)", key, numValidHashes, conflictingHashes, spotCheckHit)
		validationNeeded = true
	}

	finalStatus := "Invalid" // Default to Invalid

	if !validationNeeded {
		finalStatus = "Valid"
		logrus.Infof("Proof accepted for key %s based on consensus.", key)
	} else {
		if numValidHashes == 0 {
			logrus.Warnf("Validation failed for key %s: No valid hashes received among timely responses.", key)
			finalStatus = "Invalid"
		} else {
			logrus.Debugf("Calculating expected proof hash for key %s.", key)
			expectedHash := validation.CreatProofHash(seed, cid)

			if expectedHash == "" {
				logrus.Errorf("Failed to generate expected proof hash for key %s during validation. Marking Invalid.", key)
				finalStatus = "Invalid"
			} else {
				if expectedHash == firstValidHash {
					logrus.Infof("Proof validation successful for key %s: Expected hash matches received valid hash (%s).", key, expectedHash)
					finalStatus = "Valid"
				} else {
					logrus.Warnf("Proof validation failed for key %s: Hash mismatch. Expected %s, received consensus hash %s.", key, expectedHash, firstValidHash)
					finalStatus = "Invalid"
				}
			}
		}
	}

	// --- Record the final status ---
	// Use the passed targetName directly
	if targetName == "" { // Add a fallback just in case
		targetName = "Consensus"
	}

	localdata.SetStatus(seed, cid, finalStatus, targetName, startTime, 0) // Use updated SetStatus, pass 0 duration
	logrus.Infof("Final status for key %s set to %s for target %s", key, finalStatus, targetName)
}

// SendProof
// This is the function that sends the proof of access to the validation node
func SendProof(req Request, validationHash string, salt string, user string, ws *websocket.Conn) {
	data := map[string]string{
		"type": TypeProofOfAccess,
		"hash": validationHash,
		"seed": salt,
		"user": user,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		logrus.Errorf("Error encoding ProofOfAccess JSON: %v", err)
		return
	}
	wsPeers := localdata.WsPeers[req.User]
	nodeType := localdata.NodeType
	if wsPeers == req.User && nodeType == 1 {
		WsMutex.Lock()
		err = ws.WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			logrus.Errorf("Error writing ProofOfAccess message to WebSocket for %s: %v", req.User, err)
		}
		WsMutex.Unlock()
	} else if localdata.UseWS == true && nodeType == 2 {
		WsMutex.Lock()
		err = ws.WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			logrus.Errorf("Error writing ProofOfAccess message to WebSocket validator %s: %v", req.User, err)
		}
		WsMutex.Unlock()
	} else {
		pubsub.Publish(string(jsonData), req.User)
	}
}
