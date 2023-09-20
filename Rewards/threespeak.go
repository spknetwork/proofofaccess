package Rewards

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"strings"
)

func ThreeSpeak() {
	hashSet := make(map[string]struct{})
	for skip := 0; skip <= 2000; skip += 40 {
		resp, err := http.Get(fmt.Sprintf("https://3speak.tv/api/new/more?skip=%d", skip))
		if err != nil {
			fmt.Println(err)
			continue
		}

		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Println(err)
			continue
		}

		var apiResponse APIResponse
		err = json.Unmarshal(body, &apiResponse)
		if err != nil {
			fmt.Println(err)
			continue
		}

		for _, rec := range apiResponse.Recommended {
			hash := strings.TrimPrefix(strings.TrimSuffix(rec.VideoV2, "/manifest.m3u8"), "ipfs://")
			if hash != `` {
				hashSet[hash] = struct{}{}
			}
		}
	}

	localdata.ThreeSpeakVideos = make([]string, 0, len(hashSet))
	for hash := range hashSet {
		localdata.ThreeSpeakVideos = append(localdata.ThreeSpeakVideos, hash)
	}
}

func PinVideos(gb int) error {
	fmt.Println("Pinning videos")
	sh := ipfs.Shell
	const GB = 1024 * 1024 * 1024
	limit := int64(gb * GB)
	totalPinned := int64(0)

	// Convert ThreeSpeakVideos to a map for quicker lookups
	videoMap := make(map[string]bool)
	for _, cid := range localdata.ThreeSpeakVideos {
		videoMap[cid] = true
	}

	// Check all the currently pinned CIDs
	fmt.Println("Checking currently pinned CIDs")
	allPinsData, _ := sh.Pins()

	// Map the allPins to only CIDs
	allPins := make(map[string]bool)
	for cid := range allPinsData {
		allPins[cid] = true
	}

	for cid, pinInfo := range allPinsData {
		// Filter only the direct pins
		if pinInfo.Type == "recursive" {
			if videoMap[cid] {
				fmt.Println("CID is in the video list", cid)
				// If the CID is in the video list, get its size and add to totalPinned
				stat, err := sh.ObjectStat(cid)
				if err != nil {
					fmt.Printf("Error getting stats for CID %s: %s\n", cid, err)
					continue
				}
				size := int64(stat.CumulativeSize)
				totalPinned += size
				fmt.Println("Total pinned: ", totalPinned)
			} else {
				// If the CID is not in the video list, unpin it
				fmt.Println("Unpinning CID: ", cid)
				if err := sh.Unpin(cid); err != nil {
					fmt.Printf("Error unpinning CID %s: %s\n", cid, err)
					continue
				}
				fmt.Println("Unpinned CID: ", cid)
			}
		}
	}
	fmt.Println("Total pinned: ", totalPinned)

	// Pin videos from ThreeSpeak until the limit is reached
	for _, cid := range localdata.ThreeSpeakVideos {
		if totalPinned >= limit {
			fmt.Println("Total pinned is greater than limit")
			break
		}
		fmt.Println("Getting stats for CID: ", cid)
		// If CID isn't already in allPins, then it's not pinned
		if _, pinned := allPins[cid]; !pinned {
			stat, err := sh.ObjectStat(cid)
			if err != nil {
				fmt.Printf("Error getting stats for CID %s: %s\n", cid, err)
				continue
			}

			size := int64(stat.CumulativeSize)
			fmt.Println("Size: ", size)
			fmt.Println("Total pinned: ", totalPinned)
			fmt.Println("Limit: ", limit)
			if totalPinned+size-1000000 <= limit {
				fmt.Println("Pinning CID: ", cid)
				if err := sh.Pin(cid); err != nil {
					fmt.Printf("Failed to pin CID %s: %s\n", cid, err)
					continue
				}
				totalPinned += size
				// Once pinned, add it to the allPins
				allPins[cid] = true
				fmt.Println("Pinned CID: ", cid)
			}
		} else {
			fmt.Println("CID is already pinned")
		}
	}

	return nil
}
