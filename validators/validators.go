package validators

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"proofofaccess/localdata"
	"proofofaccess/messaging"
	"proofofaccess/proofcrypto"
	"time"
)

type Market struct {
	Domain string `json:"domain"`
}

type Markets struct {
	Node map[string]Market `json:"node"`
}

func GetValidators(validators string) {
	resp, err := http.Get("https://spkinstant.hivehoneycomb.com/markets")
	if err != nil {
		fmt.Printf("Error fetching data: %s\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %s\n", err)
		return
	}

	var markets Markets
	err = json.Unmarshal(body, &markets)
	if err != nil {
		fmt.Printf("Error unmarshalling JSON: %s\n", err)
		return
	}

	for nodeName, marketInfo := range markets.Node {
		localdata.ValidatorNames = append(localdata.ValidatorNames, nodeName)
		localdata.ValidatorAddress[nodeName] = marketInfo.Domain
	}
}
func ConnectToValidators(ctx context.Context, nodeType *int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if *nodeType == 2 {
				for _, name := range localdata.ValidatorNames {
					fmt.Println("Connecting to validator: ", name)
					salt, _ := proofcrypto.CreateRandomHash()
					messaging.SendPing(salt, name)
				}
				time.Sleep(120 * time.Second)
			}
		}
	}
}
