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

	"github.com/sirupsen/logrus"
)

type Service struct {
	Domain   string `json:"a"` // Domain
	NodeName string `json:"b"` // NodeName
}

type Services struct {
	Services []map[string]Service `json:"services"`
}

func GetValidators(validatorsUrl string) {
	logrus.Infof("Fetching validators from: %s", validatorsUrl)
	resp, err := http.Get(validatorsUrl)
	if err != nil {
		logrus.Errorf("Error fetching validators from %s: %v", validatorsUrl, err)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logrus.Errorf("Error reading validator response body from %s: %v", validatorsUrl, err)
		return
	}

	var services Services
	err = json.Unmarshal(body, &services)
	if err != nil {
		logrus.Errorf("Error unmarshalling validator JSON from %s: %v", validatorsUrl, err)
		return
	}

	var loadedValidators []string // Collect data to log outside lock
	localdata.Lock.Lock()
	// defer localdata.Lock.Unlock() // Unlock manually before logging
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
			// logrus.Debugf("Loaded validator: %s (%s)", service.NodeName, service.Domain) // Log outside lock
			loadedValidators = append(loadedValidators, fmt.Sprintf("%s (%s)", service.NodeName, service.Domain)) // Collect info
		}
	}
	// Deduplicate before releasing lock
	localdata.ValidatorNames = localdata.RemoveDuplicates(localdata.ValidatorNames)
	localdata.Lock.Unlock()

	// Log collected info outside the lock
	if len(loadedValidators) > 0 {
		logrus.Debugf("Loaded validators: %s", strings.Join(loadedValidators, ", "))
	}
}

func ConnectToValidators(ctx context.Context, nodeType *int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if *nodeType == 2 {
				localdata.Lock.Lock()
				validatorNamesCopy := make([]string, len(localdata.ValidatorNames))
				copy(validatorNamesCopy, localdata.ValidatorNames)
				localdata.Lock.Unlock()

				for _, name := range validatorNamesCopy {
					if localdata.GetNodeName() != name {
						salt, err := proofcrypto.CreateRandomHash()
						if err != nil {
							logrus.Errorf("Error creating random hash for ping to %s: %v", name, err)
							continue
						}
						messaging.SendPing(salt, name, nil)
					}
				}
				time.Sleep(120 * time.Second)
			}
		}
	}
}
