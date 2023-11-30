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
	"strings"
	"time"
)

type Service struct {
	Domain   string `json:"a"` // Domain
	NodeName string `json:"b"` // NodeName
}

type Services struct {
	Services []map[string]Service `json:"services"`
}

func GetValidators(validators string) {
	resp, err := http.Get(validators)
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

	var services Services
	err = json.Unmarshal(body, &services)
	if err != nil {
		fmt.Printf("Error unmarshalling JSON: %s\n", err)
		return
	}

	for _, serviceMap := range services.Services {
		for _, service := range serviceMap {
			if strings.HasPrefix(service.Domain, "https://") {
				service.Domain = "wss://" + strings.TrimPrefix(service.Domain, "https://")
			}
			if strings.HasSuffix(service.Domain, "/") {
				service.Domain = strings.TrimSuffix(service.Domain, "/")
			}
			localdata.ValidatorNames = append(localdata.ValidatorNames, service.NodeName)
			localdata.ValidatorAddress[service.NodeName] = service.Domain
			fmt.Println("Service:", service)
		}
	}
}
func ConnectToValidators(ctx context.Context, nodeType *int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if *nodeType == 2 {
				fmt.Println("Connecting to validators")
				for _, name := range localdata.ValidatorNames {
					if localdata.GetNodeName() != name {
						fmt.Println("Connecting to validator: ", name)
						salt, _ := proofcrypto.CreateRandomHash()
						messaging.SendPing(salt, name, localdata.WsValidators[name])
					}
				}
				time.Sleep(120 * time.Second)
			}
		}
	}
}
