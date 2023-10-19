package connection

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"os"
	"os/signal"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"proofofaccess/proofcrypto"
	"sync"
	"time"
)

var (
	interrupt = make(chan os.Signal, 1)
	done      = make(chan struct{})
)
var mu sync.Mutex

func IsConnectionOpen(conn *websocket.Conn) bool {
	fmt.Println("Checking if connection is open")
	var writeWait = 1 * time.Second

	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		log.Println("SetWriteDeadline failed:", err)
		return false
	}

	// Write the ping message to the connection
	if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
		log.Println("WriteMessage failed:", err)
		return false
	}

	// Reset the write deadline
	if err := conn.SetWriteDeadline(time.Time{}); err != nil {
		log.Println("Resetting WriteDeadline failed:", err)
		return false
	}
	fmt.Println("Connection is open")
	return true
}

func CheckSynced(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			for _, peerName := range localdata.PeerNames {
				fmt.Println("Checking if synced with", peerName)
				if localdata.WsPeers[peerName] == peerName {
					peerWs := localdata.WsClients[peerName]
					fmt.Println("Checking if synced with2", peerName)
					if IsConnectionOpen(peerWs) == false {
						fmt.Println("Connection to validator", peerName, "lost")
						localdata.Lock.Lock()
						localdata.WsPeers[peerName] = ""
						localdata.WsClients[peerName] = nil
						localdata.NodesStatus[peerName] = "Disconnected"
						localdata.Lock.Unlock()
						newPeerNames := make([]string, 0, len(localdata.PeerNames)-1)
						for _, pn := range localdata.PeerNames {
							if pn != peerName {
								newPeerNames = append(newPeerNames, pn)
								fmt.Println("Removing", peerName, "from peerNames")
							}
						}
						localdata.Lock.Lock()
						localdata.PeerNames = newPeerNames
						localdata.Lock.Unlock()

					}
				} else {
					fmt.Println("Connection to validator", peerName, "lost")
					// Get the start time from the seed
					start := localdata.PingTime[peerName]
					// Get the current time
					elapsed := time.Since(start)
					if elapsed.Seconds() > 121 {
						peerN := localdata.PeerNames
						newPeerNames := make([]string, 0, len(localdata.PeerNames)-1)
						for _, pn := range peerN {
							if pn != peerName {
								newPeerNames = append(newPeerNames, pn)
							}
						}
						fmt.Println("Removing", peerName, "from peerNames")
						localdata.Lock.Lock()
						localdata.PeerNames = newPeerNames
						localdata.Lock.Unlock()
						fmt.Println("Removing2", peerName, "from wsPeers")
					}
				}
			}
			time.Sleep(10 * time.Second)
		}
	}
}

func StartWsClient(name string) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	var c *websocket.Conn
	var isConnected bool
	var err error

	// client reading messages
	go func() {
		for {
			if isConnected {
				localdata.Lock.Lock()
				validatorWs := localdata.WsValidators[name]
				_, message, err := validatorWs.ReadMessage()
				localdata.Lock.Unlock()
				if err != nil {
					log.Println("read:", err)
					fmt.Println("Connection lost. Reconnecting...")
					isConnected = false
					continue
				}
				go messaging.HandleMessage(string(message))
				//fmt.Println("Client recv: ", string(message))
			} else {
				log.Println("Connection is not established.")
				time.Sleep(1 * time.Second) // Sleep for a second before next reconnection attempt
			}
		}
	}()

	// Connection or reconnection loop
	for {
		for {
			if !isConnected {
				c, _, err = websocket.DefaultDialer.Dial(localdata.ValidatorAddress[name], nil)
				if err != nil {
					log.Println("dial:", err)
					time.Sleep(1 * time.Second)
					continue
				}
				isConnected = true
				log.Println("Connected to the server")
				salt, _ := proofcrypto.CreateRandomHash()
				localdata.Lock.Lock()
				localdata.WsValidators[name] = c
				localdata.Lock.Unlock()
				fmt.Println("Connected to validator1")
				wsPing(salt, name)
			} else {
				// Ping the server to check if still connected
				validatorWs := localdata.WsValidators[name]
				messaging.WsMutex.Lock()
				err = validatorWs.WriteMessage(websocket.PingMessage, nil)
				messaging.WsMutex.Unlock()
				if err != nil {
					log.Println("write:", err)
					fmt.Println("Connection lost. Reconnecting...")
					isConnected = false
				}
			}

			select {
			case <-interrupt:
				log.Println("interrupt")
				if isConnected {
					localdata.Lock.Lock()
					validatorWs := localdata.WsValidators[name]
					err = validatorWs.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					localdata.Lock.Unlock()
					if err != nil {
						log.Println("write close:", err)
						return
					}
				}
				return
			default:
				time.Sleep(1 * time.Second)
			}
		}

		select {
		case <-interrupt:
			log.Println("interrupt")
			if isConnected {
				localdata.Lock.Lock()
				validatorWs := localdata.WsValidators[name]
				err = validatorWs.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				localdata.Lock.Unlock()
				if err != nil {
					log.Println("write close:", err)
					return
				}
			}
			return
		default:
			// Run default operations in non-blocking manner
			time.Sleep(1 * time.Second) // Added sleep here
		}
	}
}

func wsPing(hash string, name string) {
	fmt.Println("Sending Ping")
	data := map[string]string{
		"type": "PingPongPing",
		"hash": hash,
		"user": localdata.GetNodeName(),
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error encoding JSON:", err)
		return
	}
	fmt.Println("Client send: ", string(jsonData))
	validatorWs := localdata.WsValidators[name]
	err = validatorWs.WriteMessage(websocket.TextMessage, jsonData)
}
