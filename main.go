package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"proofofaccess/Rewards"
	"proofofaccess/api"
	"proofofaccess/connection"
	"proofofaccess/database"
	"proofofaccess/hive"
	"proofofaccess/honeycomb"
	"proofofaccess/ipfs"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"proofofaccess/peers"
	"proofofaccess/validators"
	"sync"
	"syscall"
	"time"

	shell "github.com/ipfs/go-ipfs-api"
	"github.com/sirupsen/logrus"
)

var (
	nodeType       = flag.Int("node", 1, "Node type 1 = validation 2 = access")
	storageLimit   = flag.Int("storageLimit", 1, "storageLimit in GB")
	username       = flag.String("username", "", "Username")
	ipfsPort       = flag.String("IPFS_PORT", "5001", "IPFS port number")
	wsPort         = flag.String("WS_PORT", "8000", "Websocket port number")
	useWS          = flag.Bool("useWS", false, "Use websocket")
	getVids        = flag.Bool("getVids", false, "Fetch 3Speak videos for rewarding")
	pinVideos      = flag.Bool("pinVideos", false, "Pin videos")
	getHiveRewards = flag.Bool("getHive", false, "Get Hive rewards")
	useHoneycomb   = flag.Bool("honeycomb", false, "Use honeycomb")
	honeycombApi   = flag.String("url", "", "Honeycomb API URL")
	validatorsApi  = flag.String("validators", "https://spktest.dlux.io/services/VAL", "Validators URL")
	threeSpeakNode = flag.Bool("threeSpeak", false, "3Speak node")
	CID, Hash      string
	log            = logrus.New()
	newPins        = false
)
var mu sync.Mutex

func main() {
	flag.Parse()
	log.SetLevel(logrus.DebugLevel)
	ipfs.Shell = shell.NewShell("localhost:" + *ipfsPort)

	// Wait for IPFS to be ready before proceeding
	log.Info("Waiting for IPFS connection...")
	for i := 0; i < 30; i++ { // Try for 30 seconds
		if ipfs.Shell.IsUp() {
			log.Info("IPFS connection established")
			break
		}
		if i == 29 {
			log.Fatal("Failed to connect to IPFS after 30 seconds. Please ensure IPFS daemon is running on localhost:" + *ipfsPort)
		}
		log.Debugf("IPFS not ready, retrying in 1 second... (attempt %d/30)", i+1)
		time.Sleep(1 * time.Second)
	}

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
	log.Infof("Initializing node: %s (Type: %d)", *username, *nodeType)

	// Only storage nodes need database and content management
	if *nodeType == 2 {
		database.Init(*nodeType)
		ipfs.IpfsPeerID()

		if *getHiveRewards {
			log.Debug("Getting Hive rewards...")
			localdata.HiveRewarded = hive.GetHiveSent()
		}
		if *getVids {
			log.Debug("Getting 3Speak videos...")
			Rewards.ThreeSpeak()
			if *pinVideos {
				log.Debug("Pinning and unpinning videos")
				go Rewards.PinVideos(*storageLimit)
			}
			go ipfs.SaveRefs(localdata.ThreeSpeakVideos)
		}
		if *useHoneycomb {
			var url = ""
			if *honeycombApi == "" {
				url = "https://spktest.dlux.io/list-contracts"
			} else {
				url = *honeycombApi
			}
			log.Infof("Getting Honeycomb CIDs from %s", url)
			cids, err := honeycomb.GetCIDsFromAPI(url)
			if err != nil {
				log.Errorf("Error getting Honeycomb CIDs: %v", err)
			} else {
				localdata.Lock.Lock()
				localdata.HoneycombContractCIDs = cids
				localdata.Lock.Unlock()
				go ipfs.SaveRefs(cids)
			}
		}
	}

	if *nodeType == 1 {
		log.Info("Starting as Validation Node - lightweight challenge coordinator")
		go messaging.PubsubHandler(ctx)
		// REMOVED: Content management - validators don't manage content
		// REMOVED: Database initialization - validators are stateless
	} else {
		log.Info("Starting as Access Node")
		go peers.FetchPins()
	}

	if *nodeType == 2 {
		validators.GetValidators(*validatorsApi)
		if *useWS {
			localdata.UseWS = *useWS
			log.Info("Connecting to validators via WebSocket")
			for _, name := range localdata.ValidatorNames {
				go connection.StartWsClient(name)
			}
		} else {
			log.Info("Connecting to validators via PubSub")
			go messaging.PubsubHandler(ctx)
			go validators.ConnectToValidators(ctx, nodeType)
		}
	}

	go api.StartAPI(ctx)

	if *nodeType == 1 {
		log.Info("Starting proof validation challenge coordinator")
		go Rewards.RunValidationChallenges(ctx)
	}

	log.Info("Initialization complete")
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
