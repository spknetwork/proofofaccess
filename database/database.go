package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dgraph-io/badger"
	"log"
	"strings"
	"time"
)

type Message struct {
	CID     string `json:"CID"`
	Seed    string `json:"seed"`
	Status  string `json:"status"`
	Name    string `json:"name"`
	Time    string `json:"time"`
	Elapsed string `json:"elapsed"`
}

// DB
// Open the database
var DB *badger.DB
var Lock = false

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

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return ErrDatabaseClosed
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
	if Lock == true {
		for Lock == true {
			time.Sleep(10 * time.Millisecond)
		}
	}
	Lock = true
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
		Lock = false
		return nil
	})
	fmt.Println("Value deleted")
	if err != nil {
		fmt.Printf("Error deleting value: %v\n", err)
		Lock = false
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
	if Lock == true {
		for Lock == true {
			time.Sleep(10 * time.Millisecond)
		}
	}
	Lock = true
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
	Lock = false
	return
}

// Update
// Update the value associated with a key
func Update(key []byte, value []byte) {
	if err := checkDatabaseOpen(); err != nil {
		fmt.Println("Error checking database open")
		log.Fatal(err)
	}

	fmt.Println("Updating value")
	if Lock == true {
		fmt.Println("Lock is true")
		for Lock == true {
			time.Sleep(10 * time.Millisecond)
		}
	}
	Lock = true
	err := DB.Update(func(txn *badger.Txn) error {
		err := txn.Set([]byte(key), []byte(value))
		if err != nil {
			log.Panic(err)
		}
		Lock = false
		return nil
	})
	fmt.Println("Value updated")
	if err != nil {
		fmt.Printf("Error updating value: %v\n", err)
		Lock = false
		return
	}
}

// Read
// Read the data from the database
func Read(key []byte) []byte {
	if Lock == true {
		for Lock == true {
			time.Sleep(10 * time.Millisecond)
		}
	}
	Lock = true
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
			Lock = false
			return nil
		}
		log.Panic(err)
	}
	val, err := item.ValueCopy(nil)
	if err != nil {
		log.Panic(err)
	}
	Lock = false
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

// GetStats
// Reads all the Stats from the database
func GetStats(page int) []Message {
	if err := checkDatabaseOpen(); err != nil {
		log.Fatal(err)
	}

	const pageSize = 50 // define the number of results per page
	skipEntries := (page - 1) * pageSize

	var messages []Message

	err := DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 10
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("Stats")
		count := 0

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if count < skipEntries { // skip entries that are not in the current page
				count++
				continue
			}
			if count >= skipEntries+pageSize { // stop iterating after reaching the page limit
				break
			}

			item := it.Item()
			k := item.Key()
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			if strings.HasPrefix(string(k), "Stats") {
				var message Message
				err := json.Unmarshal(v, &message)
				if err != nil {
					log.Println("Error decoding JSON:", err)
					continue
				}
				messages = append(messages, message)
			}

			count++
		}

		return nil
	})
	if err != nil {
		log.Printf("Error reading all stats: %v\n", err)
	}

	return messages
}
