package api

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"net/http"
	"time"
)

var (
	CID = ""
	log = logrus.New()
)

const (
	pingTimeout       = 5 * time.Second
	maxPingAttempts   = 3
	validationTimeout = 10 * time.Second
	wsError           = "Error"
	wsRequestingProof = "RequestingProof"
	wsProofReceived   = "ProofReceived"
	wsValidating      = "Validating"
)

type Response struct {
	IP string `json:"ip"`
}

func StartAPI(ctx context.Context) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New() // Use gin.New() instead of gin.Default()

	// This will ensure that panics are still caught and returned as 500 error
	r.Use(gin.Recovery())

	// Serve the index.html file on the root route
	r.StaticFile("/", "./public/index.html")
	r.StaticFile("/node-stats", "./public/node-stats.html")
	r.StaticFile("/stats", "./public/stats.html")
	// Handle the API request
	r.GET("/validate", handleValidate)
	r.POST("/shutdown", handleShutdown)
	r.GET("/getstats", handleStats)
	r.POST("/write", handleWrite)
	r.GET("/read", handleRead)
	r.GET("/update", handleUpdate)
	r.GET("/delete", handleDelete)
	r.GET("/messaging", handleMessaging)
	r.Static("/public", "./public")
	// Handle the DNS lookup API request
	r.GET("/get-ip", getIPHandler)
	r.GET("/get-stats", getStatsHandler)
	// Start the server
	server := &http.Server{
		Addr:    ":8000",
		Handler: r,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	<-ctx.Done()

	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctxShutdown); err != nil {
		log.Fatalf("server shutdown failed: %s\n", err)
	}
}
