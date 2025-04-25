package peers

import (
	"encoding/json"
	"proofofaccess/database"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"sync"

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

	logrus.Info("Fetching pins...")
	allPins, err := ipfs.Shell.Pins()
	if err != nil {
		logrus.Errorf("Error fetching pins: %v", err)
		return
	}

	for _, cid := range messaging.PinFileCids {
		delete(allPins, cid)
	}
	ipfs.AllPins = allPins

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
			if !ipfs.IsPinnedInDB(key) {
				savedRefs, err := ipfs.Refs(key)
				if err != nil {
					logrus.Errorf("Error getting refs for %s: %v", key, err)
					return
				}

				// Marshal JSON outside the lock
				refsBytes, err := json.Marshal(savedRefs)
				if err != nil {
					// Log error and return (no lock held yet)
					logrus.Errorf("Error marshaling refs for %s: %v", key, err)
					return
				}

				localdata.Lock.Lock()
				localdata.SavedRefs[key] = savedRefs
				localdata.Lock.Unlock() // Release lock before DB save

				database.Save([]byte("refs"+key), refsBytes)
			}
		}(key)
	}

	wg.Wait()

	localdata.Lock.Lock()
	localdata.PeerSize[localdata.NodeName] = PeerSize
	localdata.SyncedPercentage = 100
	localdata.Lock.Unlock()

	logrus.Infof("Pin fetch and processing complete. Total size: %d", PeerSize)
}
