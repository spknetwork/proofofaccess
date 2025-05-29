package connection

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"proofofaccess/proofcrypto"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
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
			localdata.Lock.Lock()
			wsPeersCopy := make([]string, 0, len(localdata.WsPeers))
			for peerName := range localdata.WsPeers {
				wsPeersCopy = append(wsPeersCopy, peerName)
			}
			localdata.Lock.Unlock()

			for _, peerName := range wsPeersCopy {
				localdata.Lock.Lock()
				isRegisteredPeer := localdata.WsPeers[peerName] == peerName
				peerWs := localdata.WsClients[peerName]
				pstartTime := localdata.PingTime[peerName]
				localdata.Lock.Unlock()

				if isRegisteredPeer {
					if peerWs == nil || !IsConnectionOpen(peerWs) {
						logrus.Debugf("WebSocket connection to peer %s lost or failed check.", peerName)
						localdata.Lock.Lock()
						delete(localdata.WsPeers, peerName)
						delete(localdata.WsClients, peerName)
						localdata.NodesStatus[peerName] = "Disconnected"
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
					if !pstartTime.IsZero() {
						elapsed := time.Since(pstartTime)
						if elapsed.Seconds() > 121 {
							logrus.Debugf("Peer %s inactive (last ping %v ago), removing.", peerName, elapsed)
							localdata.Lock.Lock()
							peerN := localdata.PeerNames
							newPeerNames := make([]string, 0, len(peerN)-1)
							for _, pn := range peerN {
								if pn != peerName {
									newPeerNames = append(newPeerNames, pn)
								}
							}
							localdata.PeerNames = newPeerNames
							delete(localdata.WsPeers, peerName)
							delete(localdata.PingTime, peerName)
							localdata.Lock.Unlock()
						}
					}
				}
			}
			time.Sleep(10 * time.Second)
		}
	}
}

func IsConnectionOpen(conn *websocket.Conn) bool {
	if conn == nil {
		return false
	}
	var writeWait = 1 * time.Second

	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		logrus.Debugf("SetWriteDeadline failed for WS ping check: %v", err)
		return false
	}

	if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
		logrus.Debugf("Write PingMessage failed for WS ping check: %v", err)
		return false
	}

	if err := conn.SetWriteDeadline(time.Time{}); err != nil {
		logrus.Debugf("Resetting WriteDeadline failed after WS ping check: %v", err)
		return false
	}
	return true
}

func StartWsClient(name string) {
	if localdata.UseWS == false {
		logrus.Info("Skipping WebSocket client connection: UseWS is false")
		return
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	maxBackoff := 60 * time.Second // Max wait 1 minute
	baseBackoff := 1 * time.Second // Start with 1 second
	currentBackoff := baseBackoff

	for {
		err := connectAndListen(name, interrupt)
		if err != nil {
			logrus.Debugf("WebSocket client connection error for %s: %v. Retrying in %v...", name, err, currentBackoff)

			jitter := time.Duration(rand.Int63n(int64(currentBackoff))) - (currentBackoff / 2)
			waitTime := currentBackoff + jitter
			if waitTime < 0 {
				waitTime = 100 * time.Millisecond
			}

			time.Sleep(waitTime)

			currentBackoff *= 2
			if currentBackoff > maxBackoff {
				currentBackoff = maxBackoff
			}
		} else {
			logrus.Infof("WebSocket client for %s disconnected cleanly.", name)
			currentBackoff = baseBackoff
			time.Sleep(time.Second * 1)
		}
	}
}

func connectAndListen(name string, interrupt <-chan os.Signal) error {
	localdata.Lock.Lock()
	u := localdata.ValidatorAddress[name] + "/messaging"
	localdata.Lock.Unlock()
	logrus.Debugf("Attempting to connect WebSocket client to validator %s at %s", name, u)

	c, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		return fmt.Errorf("dial error for %s: %v", u, err)
	}
	logrus.Infof("WebSocket client connected to %s", u)
	defer c.Close()

	// Username verification
	err = verifyUsername(c, name)
	if err != nil {
		logrus.Errorf("Username verification failed for %s: %v", name, err)
		return fmt.Errorf("username verification failed for %s: %v", name, err)
	}
	logrus.Infof("Username verification successful for %s", name)

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
					logrus.Errorf("WebSocket read error for %s: %v", name, err)
				} else {
					logrus.Debugf("WebSocket closed for %s: %v", name, err)
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

	salt, err := proofcrypto.CreateRandomHash()
	if err != nil {
		logrus.Errorf("Error creating random hash for initial wsPing to %s: %v", name, err)
		return err
	}
	wsPing(salt, name, c)

	for {
		select {
		case <-done:
			return nil
		case <-ticker.C:
			err := c.SetWriteDeadline(time.Now().Add(writeWait))
			if err != nil {
				logrus.Errorf("WebSocket SetWriteDeadline error before ping for %s: %v", name, err)
				return err
			}
			if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
				logrus.Errorf("WebSocket write ping error for %s: %v", name, err)
				return fmt.Errorf("write ping: %v", err)
			}
		case <-interrupt:
			logrus.Info("Interrupt received, closing WebSocket connection for ", name)
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				logrus.Errorf("WebSocket write close error for %s: %v", name, err)
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

// verifyUsername sends an identity request and waits for a response to verify the expected username
func verifyUsername(conn *websocket.Conn, expectedUsername string) error {
	// Send identity request
	identityRequest := map[string]string{
		"type": "IdentityRequest",
		"user": localdata.GetNodeName(),
	}

	jsonData, err := json.Marshal(identityRequest)
	if err != nil {
		return fmt.Errorf("error encoding identity request: %v", err)
	}

	err = conn.WriteMessage(websocket.TextMessage, jsonData)
	if err != nil {
		return fmt.Errorf("error sending identity request: %v", err)
	}

	// Set read deadline for response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Wait for response
	_, message, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("error reading identity response: %v", err)
	}

	// Parse response
	var response map[string]interface{}
	err = json.Unmarshal(message, &response)
	if err != nil {
		return fmt.Errorf("error parsing identity response: %v", err)
	}

	// Check if it's an identity response
	if responseType, ok := response["type"].(string); !ok || responseType != "IdentityResponse" {
		return fmt.Errorf("expected IdentityResponse, got: %v", responseType)
	}

	// Check username
	if username, ok := response["user"].(string); !ok || username != expectedUsername {
		return fmt.Errorf("username mismatch: expected %s, got %s", expectedUsername, username)
	}

	// Reset read deadline
	conn.SetReadDeadline(time.Time{})

	return nil
}

func wsPing(hash string, name string, c *websocket.Conn) {
	data := map[string]string{
		"type": "PingPongPing",
		"hash": hash,
		"user": localdata.GetNodeName(),
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		logrus.Errorf("Error encoding wsPing JSON for %s: %v", name, err)
		return
	}
	err = c.WriteMessage(websocket.TextMessage, jsonData)
	if err != nil {
		logrus.Errorf("Error writing wsPing message for %s: %v", name, err)
	}
}
