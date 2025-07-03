package peers

import (
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var lock sync.Mutex
var Pins = make(map[string]interface{})
var NewPins = make(map[string]interface{})

func FetchPins() {
	localdata.Lock.Lock()
	Pins = NewPins
	var PeerSize = 0
	localdata.Lock.Unlock()

	logrus.Debug("Fetching pins...")
	allPins, err := ipfs.Shell.Pins()
	if err != nil {
		logrus.Errorf("Error fetching pins: %v", err)
		return
	}

	for _, cid := range messaging.PinFileCids {
		delete(allPins, cid)
	}

	localdata.Lock.Lock()
	NewPins = make(map[string]interface{})
	localdata.Lock.Unlock()

	for key, pinInfo := range allPins {
		if pinInfo.Type == "recursive" {
			localdata.Lock.Lock()
			NewPins[key] = key
			localdata.Lock.Unlock()
		}
	}

	var wg sync.WaitGroup

	// Add rate limiting to prevent overwhelming IPFS
	semaphore := make(chan struct{}, 5) // Limit to 5 concurrent operations

	for key := range NewPins {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			size, _ := ipfs.FileSize(key)
			localdata.Lock.Lock()
			PeerSize += size
			localdata.Lock.Unlock()
			// Get refs for pinned content
			savedRefs, err := ipfs.Refs(key)
			if err != nil {
				logrus.Debugf("Skipping refs for CID %s in FetchPins (may not exist): %v", key, err)
				return
			}

			localdata.Lock.Lock()
			localdata.SavedRefs[key] = savedRefs
			localdata.Lock.Unlock()
		}(key)
	}

	wg.Wait()

	localdata.Lock.Lock()
	localdata.PeerSize[localdata.NodeName] = PeerSize
	localdata.SyncedPercentage = 100
	localdata.Lock.Unlock()

	logrus.Debugf("Pin fetch and processing complete. Total size: %d", PeerSize)
}

func GetPins() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		<-ticker.C
		logrus.Debug("Fetching pins...")
		getPeerPins()
	}
}

func getPeerPins() {
	localdata.Lock.Lock()
	Pins = NewPins
	var PeerSize = 0
	localdata.Lock.Unlock()

	logrus.Debug("Fetching pins...")
	allPins, err := ipfs.Shell.Pins()
	if err != nil {
		logrus.Errorf("Error fetching pins: %v", err)
		return
	}

	for _, cid := range messaging.PinFileCids {
		delete(allPins, cid)
	}

	localdata.Lock.Lock()
	NewPins = make(map[string]interface{})
	localdata.Lock.Unlock()

	for key, pinInfo := range allPins {
		if pinInfo.Type == "recursive" {
			localdata.Lock.Lock()
			NewPins[key] = key
			localdata.Lock.Unlock()
		}
	}

	var wg sync.WaitGroup

	for key := range NewPins {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			size, _ := ipfs.FileSize(key)
			localdata.Lock.Lock()
			PeerSize += size
			localdata.Lock.Unlock()
			// Get refs for pinned content
			savedRefs, err := ipfs.Refs(key)
			if err != nil {
				logrus.Errorf("Error getting refs for %s: %v", key, err)
				return
			}

			localdata.Lock.Lock()
			localdata.SavedRefs[key] = savedRefs
			localdata.Lock.Unlock()
		}(key)
	}

	wg.Wait()

	localdata.Lock.Lock()
	localdata.PeerSize[localdata.NodeName] = PeerSize
	localdata.SyncedPercentage = 100
	localdata.Lock.Unlock()

	logrus.Debugf("Pin fetch and processing complete. Total size: %d", PeerSize)
}
