package ipfs

import (
	"bytes"
	"fmt"
	ipfs "github.com/ipfs/go-ipfs-api"
	"io/ioutil"
)

var Shell = ipfs.NewLocalShell()

func Upload() {
	// Upload a file to IPFS
	fileBytes := []byte("Hello, IPFS")
	fileBuffer := bytes.NewBuffer(fileBytes)
	fileHash, err := Shell.Add(fileBuffer)
	if err != nil {
		fmt.Println("Error uploading file: ", err)
		return
	}
	fmt.Println("File Hash: ", fileHash)
}

func Download(fileHash string) (bytes.Buffer, error) {
	// Download the file from IPFS
	fmt.Println("Downloading file: ", fileHash)
	fileReader, err := Shell.Cat(fileHash)
	if err != nil {
		return bytes.Buffer{}, fmt.Errorf("error downloading file: %v", err)
	}
	defer fileReader.Close()
	fileContents, err := ioutil.ReadAll(fileReader)
	if err != nil {
		return bytes.Buffer{}, fmt.Errorf("error reading file contents: %v", err)
	}
	return *bytes.NewBuffer(fileContents), nil
}
