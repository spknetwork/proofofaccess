package honeycomb

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

type Response struct {
	Contracts map[string]map[string]Contract `json:"contracts"`
}

type Contract struct {
	DF map[string]int64 `json:"df"`
}

func GetCIDsFromAPI(url string) ([]string, error) {
	resp, err := http.Get(url)
	fmt.Println("resp", resp)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	fmt.Println("response", response)
	var cids []string
	for _, contracts := range response.Contracts {
		for _, contract := range contracts {
			for cid := range contract.DF {
				fmt.Println("Contract CID", cid)
				cids = append(cids, cid)
			}
		}
	}

	return cids, nil
}
