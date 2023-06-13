package api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net"
	"net/http"
	"proofofaccess/database"
)

func handleValidate(c *gin.Context) {
	log.Info("Entering handleValidate")
	conn, err := upgradeToWebSocket(c)
	if err != nil {
		return
	}
	defer closeWebSocket(conn)

	msg, err := readWebSocketMessage(conn)
	if err != nil {
		return
	}

	peerID, err := getPeerID(msg, conn)
	if err != nil {
		return
	}

	err = connectToPeer(peerID, conn, msg)
	if err != nil {
		return
	}

	salt := msg.SALT
	if salt == "" {
		salt, err = createRandomHash(conn)
		if err != nil {
			return
		}
	}
	proofJson, err := createProofRequest(salt, msg.CID, conn)
	if err != nil {
		return
	}

	err = sendProofRequest(salt, proofJson, msg.Name, conn)
	if err != nil {
		return
	}

	cid := msg.CID
	status, elapsed, err := waitForProofStatus(salt, cid, conn)
	if err != nil {
		return
	}
	sendWsResponse(status, status, formatElapsed(elapsed), conn)
	log.Info("Exiting handleValidate")
}
func handleStats(c *gin.Context) {
	conn, err := upgradeToWebSocket(c)
	if err != nil {
		return
	}
	defer closeWebSocket(conn)
	log.Info("Entering handleStats")
	stats(conn)
	return
}
func handleWrite(c *gin.Context) {
	key := c.PostForm("key")
	value := c.PostForm("value")
	database.Save([]byte(key), []byte(value))
	c.JSON(http.StatusOK, gin.H{
		"message": "Data saved successfully",
	})
}

func handleRead(c *gin.Context) {
	key := c.Query("key")
	value := database.Read([]byte(key))
	c.JSON(http.StatusOK, gin.H{
		"value": string(value),
	})
}

func handleUpdate(c *gin.Context) {
	key := c.Query("key")
	value := c.Query("value")
	database.Update([]byte(key), []byte(value))
	c.JSON(http.StatusOK, gin.H{
		"message": "Data Updated successfully",
	})
}

func handleDelete(c *gin.Context) {
	key := c.Query("key")
	database.Delete([]byte(key))
	c.JSON(http.StatusOK, gin.H{
		"message": "Data Deleted successfully",
	})
}

func getIPHandler(c *gin.Context) {
	domain := c.Query("domain")
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain parameter is required"})
		return
	}

	ip, err := getIP(domain)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error fetching IP for %s: %v", domain, err)})
		return
	}

	resp := Response{IP: ip}
	c.JSON(http.StatusOK, resp)
}

func getIP(domain string) (string, error) {
	ips, err := net.LookupIP(domain)
	if err != nil {
		return "", err
	}

	if len(ips) > 0 {
		return ips[0].String(), nil
	}
	return "", fmt.Errorf("No IP found for domain: %s", domain)
}
