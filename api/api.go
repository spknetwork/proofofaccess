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
	r := gin.Default()

	// Serve the index.html file on the root route
	go r.StaticFile("/", "./public/index.html")
	go r.StaticFile("/stats", "./public/stats.html")
	// Handle the API request
	go r.GET("/validate", handleValidate)
	go r.GET("/getstats", handleStats)
	go r.POST("/write", handleWrite)
	go r.GET("/read", handleRead)
	go r.GET("/update", handleUpdate)
	go r.GET("/delete", handleDelete)
	go r.Static("/public", "./public")
	// Handle the DNS lookup API request
	go r.GET("/get-ip", getIPHandler)

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
