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
	FileSize  int       // File size reported by the storage node (in bytes)
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
	logrus.Infof("=== Storage node received proof request ===")
	logrus.Infof("  CID: %s", CID)
	logrus.Infof("  Hash: %s", hash)
	logrus.Infof("  From: %s", req.User)
	logrus.Infof("  Request Type: %s", req.Type)
	logrus.Infof("  Timestamp: %s", time.Now().Format("15:04:05.000"))

	// Check if CID exists locally - for storage nodes, check actual IPFS availability
	isLocallyAvailable := ipfs.IsActuallyAvailable(CID)
	logrus.Infof("CID %s locally available: %t", CID, isLocallyAvailable)

	if isLocallyAvailable {
		logrus.Infof("CID %s found in local storage, generating proof hash", CID)
		startGeneration := time.Now()
		validationHash := validation.CreatProofHash(hash, CID)
		generationTime := time.Since(startGeneration)
		logrus.Infof("Generated proof hash %s for CID %s (took %v)", validationHash, CID, generationTime)

		if validationHash == "" {
			logrus.Errorf("Proof hash generation returned empty string for CID %s!", CID)
			logrus.Infof("Sending 'NA' response due to hash generation failure for CID %s to %s", CID, req.User)
			SendProof(req, "NA", hash, localdata.NodeName, ws)
		} else if validationHash == "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
			logrus.Errorf("Proof hash generation returned empty string hash for CID %s!", CID)
			logrus.Infof("Sending empty string hash response for CID %s to %s", CID, req.User)
			SendProof(req, validationHash, hash, localdata.NodeName, ws)
		} else {
			logrus.Infof("Sending valid proof hash response for CID %s to %s", CID, req.User)
			SendProof(req, validationHash, hash, localdata.NodeName, ws)
		}
		logrus.Infof("=== Proof response sent to %s ===", req.User)
	} else {
		logrus.Warnf("CID %s not found locally when handling proof request from %s", CID, req.User)
		logrus.Infof("Sending 'NA' response for unavailable CID %s to %s", CID, req.User)
		SendProof(req, "NA", hash, localdata.NodeName, ws)
		logrus.Infof("=== 'NA' response sent to %s ===", req.User)
	}
}

// HandleProofOfAccess
// MODIFIED: Collects responses, validation happens later via consensus.
func HandleProofOfAccess(req Request, ws *websocket.Conn) {
	receivedTime := time.Now() // Record when the response arrived
	CID := req.CID
	seed := req.Seed

	// Add detailed logging for proof response reception
	logrus.Infof("=== Validator received proof response ===")
	logrus.Infof("  From: %s", req.User)
	logrus.Infof("  CID: %s", CID)
	logrus.Infof("  Seed: %s", seed)
	logrus.Infof("  Hash: %s", req.Hash)
	logrus.Infof("  Received at: %s", receivedTime.Format("15:04:05.000"))

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
		FileSize:  req.Size, // Store the file size from the proof response
	}

	pendingProofsMutex.Lock()
	pendingProofs[key] = append(pendingProofs[key], response)
	logrus.Infof("Collected proof response for key %s from %s. Elapsed: %v. Hash: %s. Total collected: %d", key, req.User, elapsed, req.Hash, len(pendingProofs[key]))
	pendingProofsMutex.Unlock()

	logrus.Infof("=== Proof response processing complete for %s ===", req.User)
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

	// Clean up consensus processing flag after 5 minutes
	go func() {
		time.Sleep(5 * time.Minute)
		ConsensusProcessingMutex.Lock()
		delete(ConsensusProcessing, key)
		ConsensusProcessingMutex.Unlock()
		logrus.Debugf("Cleaned up consensus processing flag for key: %s", key)
	}()

	logrus.Debugf("Processing proof consensus for key: %s", key)

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
	validationTimeoutDuration := 30 * time.Second // Increased to match main validation timeout

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
	logrus.Debugf("Consensus for key %s: %d total responses, %d timely, %d valid hashes.", key, len(responses), numTimelyResponses, numValidHashes)

	// Determine if we need to calculate the expected hash
	validationNeeded := false
	spotCheckHit := false
	if !startTime.IsZero() { // Check if startTime is valid before using
		spotCheckHit = startTime.Nanosecond()%1e6 == 0
	} else {
		logrus.Warnf("Cannot perform spot check for key %s: Original request start time is zero.", key)
	}

	if spotCheckHit {
		logrus.Debugf("Spot check triggered for key %s (timestamp: %s)", key, startTime.String())
		validationNeeded = true
	} else if numValidHashes < 3 {
		logrus.Debugf("Validation needed for key %s: Fewer than 3 valid hashes (%d received).", key, numValidHashes)
		validationNeeded = true
	} else if conflictingHashes {
		logrus.Debugf("Validation needed for key %s: Conflicting valid hashes received.", key)
		validationNeeded = true
	} else if numValidHashes > 0 && !conflictingHashes && numValidHashes >= 3 {
		validationNeeded = false
		logrus.Debugf("Validation not needed for key %s: >=3 timely, matching valid hashes and no spot check.", key)
	} else {
		logrus.Warnf("Defaulting to validation needed for key %s (numValidHashes: %d, conflicting: %t, spotCheck: %t)", key, numValidHashes, conflictingHashes, spotCheckHit)
		validationNeeded = true
	}

	finalStatus := "Invalid" // Default to Invalid

	if !validationNeeded {
		finalStatus = "Valid"
		logrus.Debugf("Proof accepted for key %s based on consensus.", key)
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
					logrus.Debugf("Proof validation successful for key %s: Expected hash matches received valid hash (%s).", key, expectedHash)
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

	// Find the fastest response time (regardless of validity)
	var fastestResponseTime time.Duration = 0
	if len(timelyResponses) > 0 {
		// Find the fastest response among all timely responses
		fastestResponseTime = time.Hour // Start with a very large duration
		for _, resp := range timelyResponses {
			if resp.Elapsed < fastestResponseTime {
				fastestResponseTime = resp.Elapsed
			}
		}
		// If we didn't find any response times, use 0
		if fastestResponseTime == time.Hour {
			fastestResponseTime = 0
		}
		logrus.Debugf("Fastest response time for key %s: %v (status: %s)", key, fastestResponseTime, finalStatus)
	}

	localdata.SetStatus(seed, cid, finalStatus, targetName, startTime, fastestResponseTime) // Pass actual fastest response time
	logrus.Debugf("Final status for key %s set to %s for target %s with response time %v", key, finalStatus, targetName, fastestResponseTime)
}

// SendProof
// This is the function that sends the proof of access to the validation node
func SendProof(req Request, validationHash string, salt string, user string, ws *websocket.Conn) {
	logrus.Infof("=== Storage node sending proof response ===")
	logrus.Infof("  To: %s", req.User)
	logrus.Infof("  From: %s", user)
	logrus.Infof("  CID: %s", req.CID)
	logrus.Infof("  Salt: %s", salt)
	logrus.Infof("  Hash: %s", validationHash)
	logrus.Infof("  Timestamp: %s", time.Now().Format("15:04:05.000"))

	// Get file size for the CID
	var fileSize int
	if validationHash != "NA" {
		size, err := ipfs.FileSize(req.CID)
		if err != nil {
			logrus.Warnf("Failed to get file size for CID %s: %v", req.CID, err)
			fileSize = -1 // Indicate error getting size
		} else {
			fileSize = size
			logrus.Infof("  File Size: %d bytes", fileSize)
		}
	}

	data := map[string]interface{}{
		"type": TypeProofOfAccess,
		"hash": validationHash,
		"seed": salt,
		"user": user,
		"cid":  req.CID, // Include CID in response
		"size": fileSize, // Include file size in response
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		logrus.Errorf("Error encoding ProofOfAccess JSON: %v", err)
		return
	}

	wsPeers := localdata.WsPeers[req.User]
	nodeType := localdata.NodeType

	// Try WebSocket for validator receiving response (only if WebSocket connection exists)
	if wsPeers == req.User && nodeType == 1 && ws != nil {
		logrus.Infof("Sending proof response to validator %s via WebSocket", req.User)
		WsMutex.Lock()
		err = ws.WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			logrus.Errorf("Error writing ProofOfAccess message to WebSocket for %s: %v", req.User, err)
			// Fall back to PubSub if WebSocket fails
			logrus.Infof("WebSocket failed, falling back to PubSub for %s", req.User)
			pubsub.Publish(string(jsonData), req.User)
		}
		WsMutex.Unlock()
	} else if localdata.UseWS && nodeType == 2 && ws != nil {
		// Try WebSocket for storage node sending response (only if WebSocket connection exists)
		logrus.Infof("Sending proof response to validator %s via WebSocket (storage node)", req.User)
		WsMutex.Lock()
		err = ws.WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			logrus.Errorf("Error writing ProofOfAccess message to WebSocket validator %s: %v", req.User, err)
			// Fall back to PubSub if WebSocket fails
			logrus.Infof("WebSocket failed, falling back to PubSub for %s", req.User)
			pubsub.Publish(string(jsonData), req.User)
		}
		WsMutex.Unlock()
	} else {
		// Use PubSub (either as fallback or primary method)
		logrus.Infof("Sending proof response to validator %s via PubSub", req.User)
		pubsub.Publish(string(jsonData), req.User)
	}

	logrus.Infof("=== Proof response sent successfully to %s ===", req.User)
}

// HandleRandomChallenge
// Storage nodes handle random challenges from validators by picking their own content to prove
func HandleRandomChallenge(req Request, ws *websocket.Conn) {
	salt := req.Hash // The salt is in the hash field for random challenges
	logrus.Debugf("Storage node received random challenge from %s with salt %s", req.User, salt)

	// Storage node picks its own content to prove access to
	// This could be based on blockchain assignments, local policy, etc.
	selectedCID := selectRandomCIDForChallenge()

	if selectedCID == "" {
		logrus.Warnf("No content available for random challenge from %s", req.User)
		sendChallengeResponse(req, "NA", salt, localdata.NodeName, ws)
		return
	}

	logrus.Debugf("Storage node selected CID %s for challenge from %s", selectedCID, req.User)

	// Generate proof hash for the selected content
	if ipfs.IsActuallyAvailable(selectedCID) {
		logrus.Debugf("Generating proof for selected CID %s", selectedCID)
		validationHash := validation.CreatProofHash(salt, selectedCID)
		logrus.Debugf("Generated proof hash %s for CID %s", validationHash, selectedCID)
		sendChallengeResponse(req, validationHash, salt, localdata.NodeName, ws)
	} else {
		logrus.Warnf("Selected CID %s not actually available in IPFS, sending NA response", selectedCID)
		sendChallengeResponse(req, "NA", salt, localdata.NodeName, ws)
	}
}

// selectRandomCIDForChallenge - Storage node selects content for proof challenge
// This is where storage nodes implement their own content selection logic
func selectRandomCIDForChallenge() string {
	// Storage nodes can implement various strategies:
	// 1. Random selection from pinned content
	// 2. Blockchain-assigned content
	// 3. Content from external APIs (Honeycomb, etc.)
	// 4. Local configuration

	localdata.Lock.Lock()
	defer localdata.Lock.Unlock()

	if len(localdata.HoneycombContractCIDs) > 0 {
		// Pick a random Honeycomb CID if available
		idx := time.Now().Nanosecond() % len(localdata.HoneycombContractCIDs)
		selected := localdata.HoneycombContractCIDs[idx]
		logrus.Debugf("Storage node selected Honeycomb CID: %s", selected)
		return selected
	}

	logrus.Debug("Storage node has no content available for challenge")
	return ""
}

// sendChallengeResponse - Send challenge response back to validator
func sendChallengeResponse(req Request, validationHash string, salt string, user string, ws *websocket.Conn) {
	logrus.Debugf("Sending challenge response: Hash=%s, Salt=%s, From=%s, To=%s", validationHash, salt, user, req.User)

	data := map[string]string{
		"type": "ChallengeResponse",
		"hash": validationHash,
		"seed": salt,
		"user": user,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		logrus.Errorf("Error encoding ChallengeResponse JSON: %v", err)
		return
	}

	// Send response back to validator via PubSub
	logrus.Debugf("Sending challenge response to validator %s via PubSub", req.User)
	pubsub.Publish(string(jsonData), req.User)

	logrus.Debugf("Challenge response sent successfully to %s", req.User)
}

// HandleChallengeResponse
// Validators handle responses from storage nodes to random challenges
func HandleChallengeResponse(req Request, ws *websocket.Conn) {
	receivedTime := time.Now()
	salt := req.Seed
	hash := req.Hash

	logrus.Debugf("Validator received challenge response from %s: Hash=%s, Salt=%s", req.User, hash, salt)

	if salt == "" {
		logrus.Errorf("Received ChallengeResponse with missing Salt from %s", req.User)
		return
	}

	// Get the start time to calculate elapsed time
	start := localdata.GetTime("random-challenge", salt)
	if start.IsZero() {
		logrus.Warnf("Could not retrieve start time for challenge salt %s from %s", salt, req.User)
		return
	}
	elapsed := receivedTime.Sub(start)

	logrus.Debugf("Challenge response from %s took %v", req.User, elapsed)

	// For random challenges, we accept any non-"NA" response as valid
	// The storage node chose its own content to prove access to
	status := "Invalid"
	if hash != "" && hash != "NA" {
		status = "Valid"
		logrus.Debugf("Challenge response from %s accepted (storage node proved access to their content)", req.User)
	} else {
		logrus.Debugf("Challenge response from %s rejected (no content available)", req.User)
	}

	// Record the result
	localdata.SetStatus(salt, "random-challenge", status, req.User, start, 0)
	logrus.Debugf("Challenge result for %s set to %s", req.User, status)
}
