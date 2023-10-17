package main

import (
	"context"
	"flag"
	"fmt"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/sirupsen/logrus"
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
)

var (
	nodeType       = flag.Int("node", 1, "Node type 1 = validation 2 = access")
	storageLimit   = flag.Int("storageLimit", 1, "storageLimit in GB")
	username       = flag.String("username", "", "Username")
	ipfsPort       = flag.String("IPFS_PORT", "5001", "IPFS port number")
	wsPort         = flag.String("WS_PORT", "8000", "Websocket port number")
	useWS          = flag.Bool("useWS", false, "Use websocket")
	getVids        = flag.Bool("getVids", false, "Fetch 3Speak videos for rewarding")
	runProofs      = flag.Bool("runProofs", false, "Run proofs")
	pinVideos      = flag.Bool("pinVideos", false, "Pin videos")
	getHiveRewards = flag.Bool("getHive", false, "Get Hive rewards")
	useHoneycomb   = flag.Bool("honeycomb", false, "Use honeycomb")
	honeycombApi   = flag.String("url", "", "Honeycomb API URL")
	CID, Hash      string
	log            = logrus.New()
	newPins        = false
)
var mu sync.Mutex

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
	database.Init()
	ipfs.IpfsPeerID()
	if *getHiveRewards {
		fmt.Println("Getting Hive rewards")
		localdata.HiveRewarded = hive.GetHiveSent()
		fmt.Println("Done getting Hive rewards")
	}
	if *getVids {
		fmt.Println("Getting 3Speak videos")
		Rewards.ThreeSpeak()
		fmt.Println("Done getting 3Speak videos")
		if *pinVideos {
			fmt.Println("Pinning and unpinning videos")
			go Rewards.PinVideos(*storageLimit)
		}
		go ipfs.SaveRefs(localdata.ThreeSpeakVideos)
	}
	if *useHoneycomb {
		var url = ""
		if *honeycombApi == "" {
			url = "https://spktest.dlux.io/list-contract"
		} else {
			url = *honeycombApi
		}
		cids, err := honeycomb.GetCIDsFromAPI(url)
		localdata.Lock.Lock()
		localdata.HoneycombContractCIDs = cids
		localdata.Lock.Unlock()
		log.Error(err)
		go ipfs.SaveRefs(localdata.HoneycombContractCIDs)
	}
	if *nodeType == 1 {
		go messaging.PubsubHandler(ctx)
		go connection.CheckSynced(ctx)
		go Rewards.Update(ctx)
	} else {
		go peers.FetchPins(ctx)
	}
	if *nodeType == 2 {
		validators.GetValidators()
		if *useWS {
			localdata.UseWS = *useWS
			for _, name := range localdata.ValidatorNames {
				go connection.StartWsClient(name)
			}

		} else {
			go messaging.PubsubHandler(ctx)
			go validators.ConnectToValidators(ctx, nodeType)
		}
	}
	go api.StartAPI(ctx)

	if *runProofs {
		go Rewards.RunRewardProofs(ctx)
		go Rewards.RewardPeers()
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
