package database

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/dgraph-io/badger"
	log "github.com/sirupsen/logrus"
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
		if errors.Is(err, badger.ErrTruncateNeeded) {
			log.Warn("Database requires truncation. Opening with truncation enabled.")
			opts.Truncate = true
			DB, err = badger.Open(opts)
			if err != nil {
				log.Fatalf("Failed to open database even with truncation: %v", err)
			} else {
				log.Info("Database opened with value log truncation")
			}
		} else {
			log.Fatalf("Failed to open database: %v", err)
		}
	} else {
		log.Info("Database opened successfully.")
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
	for Lock {
		time.Sleep(10 * time.Millisecond)
	}

	Lock = true
	defer func() { Lock = false }()

	if err := checkDatabaseOpen(); err != nil {
		log.Fatalf("Database not open: %v", err)
	}

	log.Debugf("Deleting key: %s", string(key))
	err := DB.Update(func(txn *badger.Txn) error {
		err := txn.Delete(key)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				log.Warnf("Key not found during delete: %s", string(key))
				return nil
			} else {
				log.Panicf("Failed to delete key %s: %v", string(key), err)
			}
		}
		return err
	})

	if err == nil {
		log.Debugf("Key deleted successfully: %s", string(key))
	} else if !errors.Is(err, badger.ErrKeyNotFound) {
		log.Errorf("Error deleting key %s: %v", string(key), err)
	}
}

// Save
// Save the data to the database
func Save(key []byte, value []byte) {
	if err := checkDatabaseOpen(); err != nil {
		log.Fatalf("Database not open: %v", err)
	}
	log.Debugf("Saving data to database, key: %s", string(key))
	for Lock {
		time.Sleep(10 * time.Millisecond)
	}

	Lock = true
	defer func() { Lock = false }()

	txn := DB.NewTransaction(true)
	defer txn.Discard()

	err := txn.Set(key, value)
	if err != nil {
		log.Panicf("Failed to set key %s during save: %v", string(key), err)
	}

	err = txn.Commit()
	if err != nil {
		log.Panicf("Failed to commit transaction for key %s: %v", string(key), err)
	}
	log.Debugf("Data saved successfully for key: %s", string(key))
}

// Update
// Update the value associated with a key
func Update(key []byte, value []byte) {
	if err := checkDatabaseOpen(); err != nil {
		log.Errorf("Error checking database open: %v", err)
		log.Fatalf("Database not open: %v", err)
	}

	log.Debugf("Updating key: %s", string(key))
	for Lock {
		time.Sleep(10 * time.Millisecond)
	}
	Lock = true
	defer func() { Lock = false }()

	err := DB.Update(func(txn *badger.Txn) error {
		err := txn.Set(key, value)
		if err != nil {
			log.Panicf("Failed to set key %s during update: %v", string(key), err)
		}
		return err
	})

	if err != nil {
		log.Errorf("Error updating key %s: %v", string(key), err)
	} else {
		log.Debugf("Key updated successfully: %s", string(key))
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
	defer func() { Lock = false }()

	if err := checkDatabaseOpen(); err != nil {
		log.Fatalf("Database not open: %v", err)
	}
	log.Debugf("Reading key: %s", string(key))
	txn := DB.NewTransaction(false)
	defer txn.Discard()

	item, err := txn.Get(key)

	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			log.Debugf("Key not found: %s", string(key))
			return nil
		}
		log.Panicf("Failed to get key %s: %v", string(key), err)
	}
	val, err := item.ValueCopy(nil)
	if err != nil {
		log.Panicf("Failed to copy value for key %s: %v", string(key), err)
	}
	return val
}

// GetTime
// Get the time value associated with a key
func GetTime(key []byte) time.Time {
	if err := checkDatabaseOpen(); err != nil {
		log.Fatalf("Database not open: %v", err)
	}

	val := Read(key)
	if val == nil {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, string(val))
	if err != nil {
		log.Panicf("Failed to parse time for key %s: %v", string(key), err)
	}
	return t
}

// SetTime
// Set the time value associated with a key
func SetTime(key []byte, t time.Time) {
	if err := checkDatabaseOpen(); err != nil {
		log.Fatalf("Database not open: %v", err)
	}
	log.Debugf("Setting time for key: %s", string(key))
	Save(key, []byte(t.Format(time.RFC3339Nano)))
}

func GetStats(user string) []Message {
	if err := checkDatabaseOpen(); err != nil {
		log.Fatalf("Database not open: %v", err)
	}
	log.Debugf("Getting stats for user: %s", user)

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
				log.Errorf("Error copying value for key %s: %v", string(k), err)
				return err
			}

			var message Message
			var raw map[string]interface{}
			err = json.Unmarshal(v, &raw)
			if err != nil {
				log.Warnf("Error decoding JSON for key %s: %v", string(k), err)
				continue
			}

			name, ok := raw["name"].(string)
			if !ok {
				log.Warnf("Missing or invalid 'name' field for key %s", string(k))
				continue
			}

			if user == "" || user == name {
				if user != "" {
					log.Debugf("Found stats for user %s, key: %s", user, string(k))
				}
				cid, _ := raw["CID"].(string)
				seed, _ := raw["seed"].(string)
				status, _ := raw["status"].(string)
				elapsed, _ := raw["elapsed"].(string)
				timeStr, _ := raw["time"].(string)

				message.CID = cid
				message.Seed = seed
				message.Status = status
				message.Name = name
				message.Elapsed = elapsed

				t, err := time.Parse(time.RFC3339Nano, timeStr)
				if err != nil {
					log.Warnf("Error parsing time for key %s: %v", string(k), err)
					continue
				}
				message.Time = t
				messages = append(messages, message)
			}
		}
		return nil
	})
	if err != nil {
		log.Errorf("Error reading stats from database: %v", err)
	}
	log.Debugf("Finished getting stats for user: %s, found %d records", user, len(messages))
	return messages
}
func GetNetwork() []NetworkRecord {
	if err := checkDatabaseOpen(); err != nil {
		log.Fatalf("Database not open: %v", err)
	}
	log.Info("Getting network records")
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
				log.Errorf("Error copying value for network key %s: %v", string(k), err)
				return err
			}

			var message NetworkRecord
			var raw map[string]interface{}
			err = json.Unmarshal(v, &raw)
			if err != nil {
				log.Warnf("Error decoding JSON for network key %s: %v", string(k), err)
				continue
			}

			peerValue, ok := raw["Peers"].(float64)
			if !ok {
				log.Warnf("Unexpected type or missing value for Peers in key %s", string(k))
				continue
			}
			message.Peers = peerValue

			nsValue, ok := raw["NetworkStorage"].(float64)
			if !ok {
				log.Warnf("Unexpected type or missing value for NetworkStorage in key %s", string(k))
				continue
			}
			message.NetworkStorage = nsValue

			dateValue, exists := raw["date"]
			if !exists || dateValue == nil {
				log.Warnf("Date key not present or has nil value in key %s", string(k))
				continue
			}

			dateString, ok := dateValue.(string)
			if !ok {
				log.Warnf("Date value is not a string in key %s", string(k))
				continue
			}

			t, err := time.Parse(time.RFC3339Nano, dateString)
			if err != nil {
				log.Warnf("Error parsing time for network key %s: %v", string(k), err)
				continue
			}
			message.Date = t

			messages = append(messages, message)
		}

		return nil
	})
	if err != nil {
		log.Errorf("Error reading network records from database: %v", err)
	}
	log.Infof("Returning %d network records", len(messages))
	log.Debugf("Network records: %+v", messages)
	return messages
}
