package database

import (
	"errors"
	"fmt"
	"github.com/dgraph-io/badger"
	"log"
	"time"
)

// DB
// Open the database
var DB *badger.DB

// ErrDatabaseClosed is an error indicating that the database is closed
var ErrDatabaseClosed = errors.New("database is closed")

func Init() {
	var err error
	opts := badger.DefaultOptions("data/badger")

	// First attempt to open the database without truncation
	DB, err = badger.Open(opts)
	if err != nil {
		// If the error indicates that value log truncation is needed, try opening the database with truncation enabled
		if err == badger.ErrTruncateNeeded {
			opts.Truncate = true // Enable value log truncation on recovery
			DB, err = badger.Open(opts)
			if err != nil {
				log.Fatal(err)
			} else {
				log.Println("Database opened with value log truncation")
			}
		} else {
			log.Fatal(err)
		}
	}
}

func Close() {
	if DB != nil {
		err := DB.Close()
		if err != nil {
			log.Fatal(err)
		}
	}
}

func checkDatabaseOpen() error {
	if DB == nil {
		return ErrDatabaseClosed
	}
	return nil
}

// Delete
// Delete the value associated with a key
func Delete(key []byte) {
	if err := checkDatabaseOpen(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Deleting value")
	err := DB.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(key))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				fmt.Println("Key not found")
			} else {
				log.Panic(err)
			}
		}
		return nil
	})
	fmt.Println("Value deleted")
	if err != nil {
		fmt.Printf("Error deleting value: %v\n", err)
		return
	}
}

// Save
// Save the data to the database
func Save(key []byte, value []byte) {
	if err := checkDatabaseOpen(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Saving data to database")
	txn := DB.NewTransaction(true)
	defer txn.Discard()

	err := txn.Set(key, value)
	if err != nil {
		log.Panic(err)
	}

	err = txn.Commit()
	if err != nil {
		log.Panic(err)
	}
}

// Update
// Update the value associated with a key
func Update(key []byte, value []byte) {
	if err := checkDatabaseOpen(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Updating value")
	err := DB.Update(func(txn *badger.Txn) error {
		err := txn.Set([]byte(key), []byte(value))
		if err != nil {
			log.Panic(err)
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
	if err := checkDatabaseOpen(); err != nil {
		log.Fatal(err)
	}
	//fmt.Println("Reading data from database")
	txn := DB.NewTransaction(false)
	defer txn.Discard()

	item, err := txn.Get(key)

	if err != nil {
		if err == badger.ErrKeyNotFound {
			fmt.Printf("Key not found: %v\n", err)
			return nil
		}
		log.Panic(err)
	}
	val, err := item.ValueCopy(nil)
	if err != nil {
		log.Panic(err)
	}
	return val
}

// GetTime
// Get the time value associated with a key
func GetTime(key []byte) time.Time {
	if err := checkDatabaseOpen(); err != nil {
		log.Fatal(err)
	}

	val := Read(key)
	if val == nil {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, string(val))
	if err != nil {
		log.Panic(err)
	}
	return t
}

// SetTime
// Set the time value associated with a key
func SetTime(key []byte, t time.Time) {
	if err := checkDatabaseOpen(); err != nil {
		log.Fatal(err)
	}

	Save(key, []byte(t.Format(time.RFC3339Nano)))
}
