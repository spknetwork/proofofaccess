package speak

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
)

type APIResponse struct {
	Data []struct {
		Filename string `json:"filename"`
	} `json:"data"`
}

func main() {
	api := "https://3speak.tv/api/feed/more?skip=0"
	res, err := http.Get(api)
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}

	var response APIResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		panic(err)
	}

	filenames := ""
	count := 0

	for _, data := range response.Data {
		// Remove "ipfs://" prefix and ".mp4" suffix
		filename := strings.TrimPrefix(data.Filename, "ipfs://")
		filename = strings.TrimSuffix(filename, ".mp4")

		// Only count and add to the string if the filename starts with "q" or "b"
		if strings.HasPrefix(filename, "q") || strings.HasPrefix(filename, "b") {
			count++
			filenames += filename + "\n"
		}
	}

	// Output
	println("Total videos: ", count)
	println("Filenames: ", filenames)
}
