package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	shell "github.com/ipfs/go-ipfs-api"
	"os"
	"os/signal"
	"proofofaccess/Rewards"
	"proofofaccess/api"
	"proofofaccess/localdata"
	"proofofaccess/proofcrypto"
	"strconv"
	"sync"
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
	username  = flag.String("username", "", "Username")
	ipfsPort  = flag.String("IPFS_PORT", "5001", "IPFS port number")
	wsPort    = flag.String("WS_PORT", "8000", "Websocket port number")
	useWS     = flag.Bool("useWS", false, "Use websocket")
	getVids   = flag.Bool("getVids", false, "Fetch 3Speak videos for rewarding")
	runProofs = flag.Bool("runProofs", false, "Run proofs")
	CID, Hash string
	log       = logrus.New()
	newPins   = false
)

func main() {
	flag.Parse()

	ipfs.Shell = shell.NewShell("localhost:" + *ipfsPort)
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
	localdata.WsPort = *wsPort
	ipfs.IpfsPeerID()
	if *getVids {
		fmt.Println("Getting 3Speak videos")
		go Rewards.ThreeSpeak()
	}
	if *nodeType == 1 {
		database.Init()
		go pubsubHandler(ctx)
		go checkSynced(ctx)
	} else {
		go fetchPins(ctx)
	}
	if *useWS && *nodeType == 2 {
		localdata.UseWS = *useWS
		fmt.Println()
		go messaging.StartWsClient()

	} else {
		go pubsubHandler(ctx)
		go connectToValidators(ctx, nodeType)
	}
	go api.StartAPI(ctx)
	if *runProofs {
		fmt.Println("Running proofs")
	}

}
func connectToValidators(ctx context.Context, nodeType *int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if *nodeType == 2 {
				for i := 1; i <= 1; i++ {
					validatorName := "validator" + strconv.Itoa(i)
					fmt.Println("Connecting to validator: ", validatorName)
					salt, _ := proofcrypto.CreateRandomHash()

					messaging.SendPing(salt, validatorName)

				}
				time.Sleep(120 * time.Second)
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
	if ipfs.Shell != nil {
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
				messaging.HandleMessage(msg)
			}
		}
	} else {
		time.Sleep(1 * time.Second)
	}
}
func fetchPins(ctx context.Context) {
	var lock sync.Mutex // Mutex lock
	newPins := false    // Assuming this is a boolean based on your usage

	for {
		select {
		case <-ctx.Done():
			return
		default:
			lock.Lock()
			ipfs.Pins = ipfs.NewPins
			lock.Unlock()

			fmt.Println("Fetching pins...")
			allPins, err := ipfs.Shell.Pins()
			for _, cid := range messaging.PinFileCids {
				delete(allPins, cid)
			}
			ipfs.AllPins = allPins
			// Fetch all pins
			if err != nil {
				fmt.Println("Error fetching pins:", err)
				continue
			}

			lock.Lock()
			ipfs.NewPins = make(map[string]interface{})
			lock.Unlock()

			for key, pinInfo := range allPins {
				if pinInfo.Type == "recursive" {
					lock.Lock()
					ipfs.NewPins[key] = key
					lock.Unlock()
				}
			}

			// Calculate the length of the map and the number of keys not found in Pins
			mapLength := len(ipfs.NewPins)

			keysNotFound := 0

			// Create a WaitGroup to wait for the function to finish
			var wg sync.WaitGroup

			// Iterate through the keys in NewPins
			for key := range ipfs.NewPins {
				wg.Add(1)
				go func(key string) {
					defer wg.Done()
					// Check if the key exists in Pins
					lock.Lock()
					_, exists := ipfs.Pins[key]
					lock.Unlock()

					if !exists {
						size, _ := ipfs.FileSize(key)
						lock.Lock()
						localdata.PeerSize[localdata.NodeName] += size
						newPins = true
						lock.Unlock()

						// If the key doesn't exist in Pins, add it to the pinsNotIncluded map
						savedRefs, _ := ipfs.Refs(key)
						lock.Lock()
						localdata.SavedRefs[key] = savedRefs
						lock.Unlock()
						lock.Lock()
						localdata.SyncedPercentage = float32(keysNotFound) / float32(mapLength) * 100
						fmt.Println("Synced: ", localdata.SyncedPercentage, "%")
						lock.Unlock()
						keysNotFound++
					}
				}(key)
			}

			wg.Wait()

			fmt.Println("Synced: ", 100)
			lock.Lock()
			localdata.SyncedPercentage = 100
			lock.Unlock()

			if newPins {
				fmt.Println("New pins found")
				messaging.SendCIDS("validator1")
				newPins = false
			}

			time.Sleep(60 * time.Second)
		}
	}
}

func checkSynced(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			for _, peerName := range localdata.PeerNames {
				if localdata.WsPeers[peerName] == peerName {
					if isConnectionOpen(localdata.WsClients[peerName]) == false {
						fmt.Println("Connection to validator", peerName, "lost")
						localdata.WsPeers[peerName] = ""
						localdata.WsClients[peerName] = nil
						newPeerNames := make([]string, 0, len(localdata.PeerNames)-1)
						for _, pn := range localdata.PeerNames {
							if pn != peerName {
								newPeerNames = append(newPeerNames, pn)
							}
						}
						localdata.PeerNames = newPeerNames
					}
				} else {
					// Get the start time from the seed
					start := localdata.PingTime[peerName]

					// Get the current time
					elapsed := time.Since(start)
					if elapsed.Seconds() > 121 {
						newPeerNames := make([]string, 0, len(localdata.PeerNames)-1)
						for _, pn := range localdata.PeerNames {
							if pn != peerName {
								newPeerNames = append(newPeerNames, pn)
							}
						}
						localdata.PeerNames = newPeerNames
					}
				}
			}
			time.Sleep(60 * time.Second)
		}

	}
}
func isConnectionOpen(conn *websocket.Conn) bool {
	var writeWait = 1 * time.Second
	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return false
	}

	// Write the ping message to the connection
	if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
		return false
	}

	// Reset the write deadline
	if err := conn.SetWriteDeadline(time.Time{}); err != nil {
		return false
	}

	return true
}
