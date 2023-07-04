package messaging

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"os"
	"os/signal"
	"proofofaccess/localdata"
	"proofofaccess/proofcrypto"
	"time"
)

var (
	interrupt = make(chan os.Signal, 1)
	done      = make(chan struct{})
)

func StartWsClient() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	var c *websocket.Conn
	var isConnected bool
	var err error

	// client reading messages
	go func() {
		for {
			if isConnected {
				_, message, err := localdata.WsValidators["Validator1"].ReadMessage()
				if err != nil {
					log.Println("read:", err)
					fmt.Println("Connection lost. Reconnecting...")
					isConnected = false
					continue
				}
				go HandleMessage(string(message))
				fmt.Println("Client recv: ", string(message))
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
				c, _, err = websocket.DefaultDialer.Dial("ws://spk.tv/messaging", nil)
				if err != nil {
					log.Println("dial:", err)
					time.Sleep(1 * time.Second)
					continue
				}
				isConnected = true
				log.Println("Connected to the server")
				salt, _ := proofcrypto.CreateRandomHash()
				localdata.WsValidators["Validator1"] = c
				fmt.Println("Connected to validator1")
				wsPing(salt)
			} else {
				// Ping the server to check if still connected
				err = localdata.WsValidators["Validator1"].WriteMessage(websocket.PingMessage, nil)
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
					err = localdata.WsValidators["Validator1"].WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
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
				err = localdata.WsValidators["Validator1"].WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
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

func wsPing(hash string) {
	data := map[string]string{
		"type": TypePingPongPing,
		"hash": hash,
		"user": localdata.GetNodeName(),
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error encoding JSON:", err)
		return
	}
	fmt.Println("Client send: ", string(jsonData))
	err = localdata.WsValidators["Validator1"].WriteMessage(websocket.TextMessage, jsonData)
}
