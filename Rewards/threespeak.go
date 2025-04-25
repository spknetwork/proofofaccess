package Rewards

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"strings"

	"github.com/sirupsen/logrus"
)

func ThreeSpeak() {
	logrus.Debug("Fetching 3Speak video list...")
	hashSet := make(map[string]struct{})
	for skip := 0; skip <= 2000; skip += 40 {
		url := fmt.Sprintf("https://3speak.tv/api/new/more?skip=%d", skip)
		resp, err := http.Get(url)
		if err != nil {
			logrus.Warnf("Error fetching 3Speak data from %s: %v", url, err)
			continue
		}

		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			logrus.Warnf("Error reading 3Speak response body from %s: %v", url, err)
			continue
		}

		var apiResponse APIResponse
		err = json.Unmarshal(body, &apiResponse)
		if err != nil {
			logrus.Errorf("Error unmarshalling 3Speak JSON from %s: %v", url, err)
			continue
		}

		for _, rec := range apiResponse.Recommended {
			hash := strings.TrimPrefix(strings.TrimSuffix(rec.VideoV2, "/manifest.m3u8"), "ipfs://")
			if hash != `` {
				hashSet[hash] = struct{}{}
			}
		}
	}

	localdata.Lock.Lock()
	localdata.ThreeSpeakVideos = make([]string, 0, len(hashSet))
	for hash := range hashSet {
		localdata.ThreeSpeakVideos = append(localdata.ThreeSpeakVideos, hash)
	}
	logrus.Debugf("Fetched %d unique 3Speak video CIDs", len(localdata.ThreeSpeakVideos))
	localdata.Lock.Unlock()
}

func PinVideos(gb int) error {
	logrus.Debug("Starting 3Speak video pinning process...")
	sh := ipfs.Shell
	const GB = 1024 * 1024 * 1024
	limit := int64(gb * GB)
	totalPinned := int64(0)

	localdata.Lock.Lock()
	videoMap := make(map[string]bool)
	for _, cid := range localdata.ThreeSpeakVideos {
		videoMap[cid] = true
	}
	localdata.Lock.Unlock()

	allPinsData, err := sh.Pins()
	if err != nil {
		logrus.Errorf("Error fetching current pins: %v", err)
		return err
	}

	allPins := make(map[string]bool)
	for cid := range allPinsData {
		allPins[cid] = true
	}

	for cid, pinInfo := range allPinsData {
		if pinInfo.Type == "recursive" {
			if videoMap[cid] {
				stat, err := sh.ObjectStat(cid)
				if err != nil {
					logrus.Warnf("Error getting stats for currently pinned CID %s: %v", cid, err)
					continue
				}
				size := int64(stat.CumulativeSize)
				totalPinned += size
			} else {
				logrus.Debugf("Unpinning CID not in 3Speak list: %s", cid)
				if err := sh.Unpin(cid); err != nil {
					logrus.Errorf("Error unpinning CID %s: %v", cid, err)
					continue
				}
			}
		}
	}
	logrus.Debugf("Current relevant pinned size after cleanup: %d bytes", totalPinned)

	localdata.Lock.Lock()
	threeSpeakVideosCopy := make([]string, len(localdata.ThreeSpeakVideos))
	copy(threeSpeakVideosCopy, localdata.ThreeSpeakVideos)
	localdata.Lock.Unlock()

	for _, cid := range threeSpeakVideosCopy {
		if totalPinned >= limit {
			logrus.Debugf("Storage limit (%d bytes) reached. Stopping pinning.", limit)
			break
		}
		if _, pinned := allPins[cid]; !pinned {
			stat, err := sh.ObjectStat(cid)
			if err != nil {
				logrus.Warnf("Error getting stats for potential pin CID %s: %v", cid, err)
				continue
			}

			size := int64(stat.CumulativeSize)
			if totalPinned+size <= limit {
				logrus.Debugf("Pinning CID %s (Size: %d bytes)", cid, size)
				if err := sh.Pin(cid); err != nil {
					logrus.Errorf("Failed to pin CID %s: %v", cid, err)
					continue
				}
				totalPinned += size
				allPins[cid] = true
			} else {
				logrus.Debugf("Skipping pin for CID %s: exceeds limit (Current: %d, Size: %d, Limit: %d)", cid, totalPinned, size, limit)
			}
		} else {
			logrus.Debugf("CID %s is already pinned.", cid)
		}
	}
	logrus.Debugf("Finished pinning process. Final pinned size: %d bytes", totalPinned)
	return nil
}
