package proofcrypto

import (
	"crypto/sha256"
	"fmt"
	"github.com/multiformats/go-multihash"
	"math/rand"
	"time"
)

// use cid hashing
func HashFile(fileContents string) string {
	hash := sha256.Sum256([]byte(fileContents))
	fmt.Printf("Hash of modified file contents: %x\n", hash)

	return fmt.Sprintf("%x", hash)
}

// create a random hash
func CreateRandomHash() string {
	source := rand.NewSource(time.Now().UnixNano())
	random := rand.New(source)
	randomNumber := random.Intn(5000000000) + 1

	hash, _ := multihash.Sum([]byte(fmt.Sprintf("%d", randomNumber)), multihash.SHA2_256, -1)
	fmt.Printf("%x\n", hash)
	return hash.B58String()
}
