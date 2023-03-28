package database

import (
	"fmt"
	"github.com/dgraph-io/badger"
	"log"
)

// DB
// Open the database
var DB, _ = badger.Open(badger.DefaultOptions("data/badger"))

// Save
// Save the data to the database
func Save(key []byte, value []byte) {
	// Create a new transaction
	txn := DB.NewTransaction(true)
	defer txn.Discard()
	// Save the data to the database
	err := txn.Set(key, value)
	if err != nil {
		log.Fatal(err)
	}
	// Commit the transaction
	err = txn.Commit()
	if err != nil {
		log.Fatal(err)
	}

}

// Update
// Update the value associated with a key
func Update(key []byte, value []byte) {
	// Update the value associated with a key
	fmt.Println("Updating value")
	err := DB.Update(func(txn *badger.Txn) error {
		err := txn.Set([]byte(key), []byte(value))
		if err != nil {
			log.Fatal(err)
		}
		return nil
	})
	fmt.Println("Value updated")
	if err != nil {
		fmt.Printf("Error updating value: %v\n", err)
		return
	}
}

// Read
// Read the data from the database
func Read(key []byte) []byte {
	// Create a new transaction
	txn := DB.NewTransaction(false)
	defer txn.Discard()
	// Read the data from the database
	item, err := txn.Get(key)

	if err != nil {
		log.Panic(err)
	}
	val, err := item.ValueCopy(nil)
	if err != nil {
		log.Fatal(err)
	}
	return val
}
