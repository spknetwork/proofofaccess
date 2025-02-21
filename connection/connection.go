package connection

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"proofofaccess/proofcrypto"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

func CheckSynced(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			for _, peerName := range localdata.WsPeers {
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

func StartWsClient(name string) {
	if localdata.UseWS == false {
		fmt.Println("Skipping WebSocket connection due to UseWS being false")
		return
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	for {
		err := connectAndListen(name, interrupt)
		if err != nil {
			log.Printf("WebSocket error: %v", err)
			time.Sleep(time.Second * 5) // Wait before attempting to reconnect
		}
	}
}

func connectAndListen(name string, interrupt <-chan os.Signal) error {
	u := localdata.ValidatorAddress[name] + "/messaging"
	log.Printf("Connecting to %s", u)

	c, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		return fmt.Errorf("dial: %v", err)
	}
	defer c.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.SetReadLimit(maxMessageSize)
		c.SetReadDeadline(time.Now().Add(pongWait))
		c.SetPongHandler(func(string) error { c.SetReadDeadline(time.Now().Add(pongWait)); return nil })
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("read error: %v", err)
				}
				return
			}
			go messaging.HandleMessage(string(message), c)
		}
	}()

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	localdata.Lock.Lock()
	localdata.WsValidators[name] = c
	localdata.Lock.Unlock()

	salt, _ := proofcrypto.CreateRandomHash()
	wsPing(salt, name, c)

	for {
		select {
		case <-done:
			return nil
		case <-ticker.C:
			c.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
				return fmt.Errorf("write ping: %v", err)
			}
		case <-interrupt:
			log.Println("Interrupt received, closing connection")
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				return fmt.Errorf("write close: %v", err)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return nil
		}
	}
}

func wsPing(hash string, name string, c *websocket.Conn) {
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
	//fmt.Println("Client send: ", string(jsonData))
	err = c.WriteMessage(websocket.TextMessage, jsonData)
	if err != nil {
		log.Printf("write ping: %v", err)
	}
}
