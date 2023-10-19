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
	CID     string    `json:"CID"`
	Seed    string    `json:"seed"`
	Status  string    `json:"status"`
	Name    string    `json:"name"`
	Time    time.Time `json:"time"`
	Elapsed string    `json:"elapsed"`
}
type NetworkRecord struct {
	Peers          float64   `json:"Peers"`
	NetworkStorage float64   `json:"NetworkStorage"`
	Date           time.Time `json:"date"`
}

// DB
// Open the database
var DB *badger.DB
var Lock = false

// ErrDatabaseClosed is an error indicating that the database is closed
var ErrDatabaseClosed = errors.New("database is closed")

func Init(nodeType int) {
	var err error
	var opts badger.Options
	if nodeType == 1 {
		opts = badger.DefaultOptions("./data/badger")
	} else {
		opts = badger.DefaultOptions("./data/badger1")
	}

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

	// fmt.Println("Updating value")
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
	// fmt.Println("Value updated")
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
			//fmt.Printf("Key not found: %v\n", err)
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

func GetStats(user string) []Message {
	if err := checkDatabaseOpen(); err != nil {
		log.Fatal(err)
	}

	var messages []Message

	err := DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 1000
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("Stats")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.Key()
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			if strings.HasPrefix(string(k), "Stats") {
				var message Message
				var raw map[string]interface{}
				err := json.Unmarshal(v, &raw)
				if err != nil {
					log.Println("Error decoding JSON:", err)
					continue
				}
				if user == "" {
					message.CID = raw["CID"].(string)
					message.Seed = raw["seed"].(string)
					message.Status = raw["status"].(string)
					message.Name = raw["name"].(string)
					message.Elapsed = raw["elapsed"].(string)

					// Parse time string to time.Time
					t, err := time.Parse(time.RFC3339Nano, raw["time"].(string))
					if err != nil {
						log.Println("Error parsing time:", err)
						continue
					}
					message.Time = t
					messages = append(messages, message)

				} else if user == raw["name"].(string) {
					fmt.Println("Found user", user)
					message.CID = raw["CID"].(string)
					message.Seed = raw["seed"].(string)
					message.Status = raw["status"].(string)
					message.Name = raw["name"].(string)
					message.Elapsed = raw["elapsed"].(string)

					// Parse time string to time.Time
					t, err := time.Parse(time.RFC3339Nano, raw["time"].(string))
					if err != nil {
						log.Println("Error parsing time:", err)
						continue
					}
					message.Time = t
					messages = append(messages, message)

				}

			}
		}

		return nil
	})
	if err != nil {
		log.Printf("Error reading all stats: %v\n", err)
	}

	return messages
}
func GetNetwork() []NetworkRecord {
	if err := checkDatabaseOpen(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Getting network records")
	var messages []NetworkRecord

	err := DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 10
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("Network")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.Key()
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			if strings.HasPrefix(string(k), "Network") {
				var message NetworkRecord
				var raw map[string]interface{}
				err := json.Unmarshal(v, &raw)
				if err != nil {
					log.Println("Error decoding JSON:", err)
					continue
				}

				if peerValue, ok := raw["Peers"].(float64); ok {
					message.Peers = peerValue
				} else {
					log.Println("Unexpected type or missing value for Peers")
					continue
				}

				if nsValue, ok := raw["NetworkStorage"].(float64); ok {
					message.NetworkStorage = nsValue
				} else {
					log.Println("Unexpected type or missing value for NetworkStorage")
					continue
				}

				dateValue, exists := raw["date"]
				if !exists || dateValue == nil {
					log.Println("Date key not present or has nil value")
					continue
				}

				dateString, ok := dateValue.(string)
				if !ok {
					log.Println("Date value is not a string")
					continue
				}

				t, err := time.Parse(time.RFC3339Nano, dateString)
				if err != nil {
					log.Println("Error parsing time:", err)
					continue
				}
				message.Date = t

				messages = append(messages, message)
			}
		}

		return nil
	})
	if err != nil {
		log.Printf("Error reading all stats: %v\n", err)
	}
	fmt.Println("Returning network records")
	fmt.Println(messages)
	return messages
}
