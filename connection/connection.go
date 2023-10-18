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
	mu.Lock()
	defer mu.Unlock()
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

	return true
}

func CheckSynced(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			for _, peerName := range localdata.PeerNames {
				if localdata.WsPeers[peerName] == peerName {
					localdata.Lock.Lock()
					peerWs := localdata.WsClients[peerName]
					localdata.Lock.Unlock()
					if IsConnectionOpen(peerWs) == false {
						fmt.Println("Connection to validator", peerName, "lost")
						localdata.Lock.Lock()
						localdata.WsPeers[peerName] = ""
						localdata.WsClients[peerName] = nil
						newPeerNames := make([]string, 0, len(localdata.PeerNames)-1)
						for _, pn := range localdata.PeerNames {
							if pn != peerName {
								newPeerNames = append(newPeerNames, pn)
							}
						}
						localdata.PeerNames = newPeerNames
						localdata.Lock.Unlock()
					}
				} else {
					// Get the start time from the seed
					localdata.Lock.Lock()
					start := localdata.PingTime[peerName]
					localdata.Lock.Unlock()
					// Get the current time
					elapsed := time.Since(start)
					if elapsed.Seconds() > 121 {
						localdata.Lock.Lock()
						peerN := localdata.PeerNames
						newPeerNames := make([]string, 0, len(localdata.PeerNames)-1)
						localdata.Lock.Unlock()
						for _, pn := range peerN {
							if pn != peerName {
								newPeerNames = append(newPeerNames, pn)
							}
						}
						localdata.Lock.Lock()
						localdata.PeerNames = newPeerNames
						localdata.Lock.Unlock()
					}
				}
			}
			time.Sleep(60 * time.Second)
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
				_, message, err := localdata.WsValidators[name].ReadMessage()
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
				localdata.WsValidators[name] = c
				fmt.Println("Connected to validator1")
				wsPing(salt, name)
			} else {
				// Ping the server to check if still connected
				localdata.Lock.Lock()
				validatorWs := localdata.WsValidators[name]
				localdata.Lock.Unlock()
				err = validatorWs.WriteMessage(websocket.PingMessage, nil)
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
					localdata.Lock.Unlock()
					err = validatorWs.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
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
				localdata.Lock.Unlock()
				err = validatorWs.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
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
	localdata.Lock.Lock()
	validatorWs := localdata.WsValidators[name]
	localdata.Lock.Unlock()
	err = validatorWs.WriteMessage(websocket.TextMessage, jsonData)
}
