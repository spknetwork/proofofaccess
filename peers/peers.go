package peers

import (
	"encoding/json"
	"fmt"
	"log"
	"proofofaccess/database"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"sync"
)

var lock sync.Mutex
var Pins = make(map[string]interface{})
var NewPins = make(map[string]interface{})

func FetchPins() {
	localdata.Lock.Lock()
	Pins = NewPins
	var PeerSize = 0
	localdata.Lock.Unlock()

	fmt.Println("Fetching pins...")
	allPins, err := ipfs.Shell.Pins()
	fmt.Println("Fetched pins")
	for _, cid := range messaging.PinFileCids {
		delete(allPins, cid)
	}
	ipfs.AllPins = allPins
	// Fetch all pins
	if err != nil {
		fmt.Println("Error fetching pins:", err)
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

	// Calculate the length of the map and the number of keys not found in Pins
	mapLength := len(NewPins)

	keysNotFound := 0

	// Create a WaitGroup to wait for the function to finish
	var wg sync.WaitGroup

	// Iterate through the keys in NewPins
	for key := range NewPins {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			// Check if the key exists in Pins
			size, _ := ipfs.FileSize(key)
			localdata.Lock.Lock()
			PeerSize += size
			//fmt.Println("Peer size: ", localdata.PeerSize[localdata.NodeName])
			localdata.Lock.Unlock()
			if !ipfs.IsPinnedInDB(key) {
				// If the key doesn't exist in Pins, add it to the pinsNotIncluded map
				savedRefs, _ := ipfs.Refs(key)
				localdata.Lock.Lock()
				localdata.SavedRefs[key] = savedRefs
				refsBytes, err := json.Marshal(savedRefs)
				if err != nil {
					log.Printf("Error: %v\n", err)
					return
				}
				database.Save([]byte("refs"+key), refsBytes)
				localdata.Lock.Unlock()
				localdata.Lock.Lock()
				localdata.SyncedPercentage = float32(keysNotFound) / float32(mapLength) * 100
				fmt.Println("Synced: ", localdata.SyncedPercentage, "%")
				localdata.Lock.Unlock()
				keysNotFound++
			}
		}(key)
	}

	wg.Wait()

	fmt.Println("Synced: ", 100)
	localdata.Lock.Lock()
	localdata.PeerSize[localdata.NodeName] = PeerSize
	localdata.SyncedPercentage = 100
	localdata.Lock.Unlock()

}
