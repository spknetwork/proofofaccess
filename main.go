package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"proofofaccess/api"
	"proofofaccess/localdata"
	"proofofaccess/proofcrypto"
	"strconv"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"proofofaccess/database"
	"proofofaccess/ipfs"
	"proofofaccess/messaging"
	"proofofaccess/pubsub"
)

var (
	nodeType  = flag.Int("node", 1, "Node type 1 = validation 2 = access")
	username  = flag.String("username", "", "Node type 1 = validation 2 = access")
	CID, Hash string
	log       = logrus.New()
)

func main() {
	flag.Parse()
	ctx, cancel := context.WithCancel(context.Background())

	setupCloseHandler(cancel)
	initialize(ctx)

	<-ctx.Done()

	log.Info("Shutting down...")

	if err := database.Close(); err != nil {
		log.Error("Error closing the database: ", err)
	}

}

func initialize(ctx context.Context) {
	localdata.SetNodeName(*username)
	localdata.NodeType = *nodeType
	ipfs.IpfsPeerID()

	database.Init()
	if *nodeType == 1 {
		go api.StartAPI(ctx)
	}

	go pubsubHandler(ctx)
	go fetchPins(ctx)
	go connectToValidators(ctx, nodeType)
}

func connectToValidators(ctx context.Context, nodeType *int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if *nodeType == 2 {
				for i := 1; i <= 2; i++ {
					validatorName := "validator" + strconv.Itoa(i)
					fmt.Println("Connecting to validator: ", validatorName)
					salt, _ := proofcrypto.CreateRandomHash()
					time.Sleep(1 * time.Second)
					messaging.PingPong(salt, validatorName)

				}
			}
		}
	}
}

func setupCloseHandler(cancel context.CancelFunc) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-signalChan
		log.Infof("Received signal: %v. Shutting down...", sig)
		cancel()
	}()
}

func pubsubHandler(ctx context.Context) {
	if pubsub.Shell != nil {
		sub, err := pubsub.Subscribe(*username)
		if err != nil {
			log.Error("Error subscribing to pubsub: ", err)
			return
		}

		log.Info("User:", *username)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := pubsub.Read(sub)
				if err != nil {
					log.Error("Error reading from pubsub: ", err)
					continue
				}
				messaging.HandleMessage(msg, nodeType)
			}
		}
	} else {
		time.Sleep(1 * time.Second)
	}
}
func fetchPins(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			ipfs.Pins = ipfs.NewPins
			allPins, err := ipfs.Shell.Pins() // Fetch all pins
			if err != nil {
				fmt.Println("Error fetching pins:", err)
				continue
			}
			ipfs.NewPins = make(map[string]interface{})
			for key, pinInfo := range allPins {
				if pinInfo.Type == "recursive" {
					ipfs.NewPins[key] = key
				}
			}

			// Calculate the length of the map and the number of keys not found in Pins
			mapLength := len(ipfs.NewPins)
			keysNotFound := 0

			// Iterate through the keys in NewPins
			for key := range ipfs.NewPins {
				// Check if the key exists in Pins

				if _, exists := ipfs.Pins[key]; !exists {
					// If the key doesn't exist in Pins, add it to the pinsNotIncluded map
					ipfs.SavedRefs[key], _ = ipfs.Refs(key)
					if localdata.Synced == false {
						fmt.Println("Synced: ", float64(keysNotFound)/float64(mapLength)*100, "%")
					}
					keysNotFound++
				}
			}
			if localdata.Synced == false {
				fmt.Println("Synced: ", 100)
				localdata.Synced = true
			}

			time.Sleep(60 * time.Second)
		}
	}
}
