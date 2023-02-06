package localdata

import "time"

var CID = ""
var Hash = ""
var Time = time.Now()
var Status = "pending"
var Elapsed = time.Duration(0)

func SaveCID(sCID string) {
	CID = sCID
}

func GetCID() string {
	return CID
}

func SaveHash(hash string) {
	Hash = hash
}

func GetHash() string {
	return Hash
}
func SaveTime() {
	Time = time.Now()
}
func GetTime() time.Time {
	return Time
}
func SetElapsed(elapsed time.Duration) {
	Elapsed = elapsed
}
func GetElapsed() time.Duration {
	return Elapsed
}

func GetStatus() string {
	return Status
}
func SetStatus(status string) {
	Status = status
}
