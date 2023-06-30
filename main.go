package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	shell "github.com/ipfs/go-ipfs-api"
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
	ipfsPort  = flag.String("IPFS_PORT", "5001", "IPFS port number")
	wsPort    = flag.String("WS_PORT", "8000", "Websocket port number")
	useWS     = flag.Bool("useWS", false, "Use websocket")
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

	if *nodeType == 1 {
		database.Init()
		go pubsubHandler(ctx)
		go checkSynced(ctx)
	} else {
		go fetchPins(ctx)
	}

	go api.StartAPI(ctx)

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

					messaging.PingPong(salt, validatorName)

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
	for {
		select {
		case <-ctx.Done():
			return
		default:
			ipfs.Pins = ipfs.NewPins
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
					size, _ := ipfs.FileSize(key)
					localdata.PeerSize[localdata.NodeName] = localdata.PeerSize[localdata.NodeName] + size
					newPins = true
					// If the key doesn't exist in Pins, add it to the pinsNotIncluded map
					localdata.SavedRefs[key], _ = ipfs.Refs(key)
					if localdata.Synced == false {

						localdata.SyncedPercentage = float32(keysNotFound) / float32(mapLength) * 100
						fmt.Println("Synced: ", localdata.SyncedPercentage, "%")
					}
					keysNotFound++
				}
			}
			if localdata.Synced == false {
				fmt.Println("Synced: ", 100)
				localdata.SyncedPercentage = 100
				if *useWS && *nodeType == 2 {
					localdata.UseWS = *useWS
					go messaging.StartWsClient()
				} else {
					go pubsubHandler(ctx)
					go connectToValidators(ctx, nodeType)
				}
			}
			if newPins == true {
				fmt.Println("New pins found")
				messaging.SendCIDS("validator1")
			}
			newPins = false
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
