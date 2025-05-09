// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// Define a struct to match the expected data structure from the endpoint
type StockData struct {
	Date     string  `json:"date"`
	Open     float64 `json:"open"`
	High     float64 `json:"high"`
	Low      float64 `json:"low"`
	Close    float64 `json:"close"`
	AdjClose float64 `json:"adjusted_close"`
	Volume   int64   `json:"volume"`
}

func (a *App) Handler(w http.ResponseWriter, r *http.Request) {
	// a.log.Log(logging.Entry{
	// 	Severity: logging.Info,
	// 	HTTPRequest: &logging.HTTPRequest{
	// 		Request: r,
	// 	},
	// 	Payload: "Structured logging example.",
	// })

	// URL of the EOD Historical API (replace with the actual endpoint)
	url := "https://eodhd.com/api/eod/VOO.US?api_token=67d249e65f7402.22787178&fmt=json&from=2020-01-01"

	// Send a GET request to the EOD Historical API
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal("Error fetching data:", err)
	}
	defer resp.Body.Close()

	// Read the body of the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Error reading response body:", err)
	}

	// Parse the JSON data into a slice of StockData
	var stockData []StockData
	err = json.Unmarshal(body, &stockData)
	if err != nil {
		log.Fatal("Error unmarshalling JSON data:", err)
	}

	// // Output the data in JSON format
	// stockDataJSON, err := json.MarshalIndent(stockData, "", "  ")
	// if err != nil {
	// 	log.Fatal("Error marshalling JSON:", err)
	// }

	// Output only the first dataset in JSON format
	if len(stockData) > 0 {
		firstStockDataJSON, err := json.MarshalIndent(stockData[0], "", "  ")
		if err != nil {
			log.Fatal("Error marshalling JSON:", err)
		}

		// Print the first record as marshaled JSON
		// fmt.Println(string(firstStockDataJSON))
		saveData(firstStockDataJSON)
		fmt.Fprint(w, string(firstStockDataJSON))
	} else {
		fmt.Println("No data found.")
	}
}

func saveData(data []byte) {
	// Check if we are running on Cloud Run (set by an environment variable)
	runningInCloudRun := os.Getenv("RUNNING_IN_CLOUD_RUN") == "true"

	var directory string

	if runningInCloudRun {
		// Cloud Run mounted volume path
		directory = "/gcs-fund-service-cache" // This is the volume path in Cloud Run
	} else {
		// Local testing directory
		directory = "./gcs-fund-service-cache" // Use a local directory for testing
	}

	// Ensure the directory exists, create it if it doesn't
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		err := os.MkdirAll(directory, os.ModePerm)
		if err != nil {
			log.Fatal("Error creating directory:", err)
		}
	}

	// Get the current time in GMT (UTC)
	currentTime := time.Now().UTC()

	// Create a unique file name for each request (e.g., using a UUID)
	// fileName := fmt.Sprintf("stock_data_%s.json", uuid.New().String())
	fileName := fmt.Sprintf("stock_data_%s.json", currentTime.Format("2006-01-02T15-04-05Z"))

	// Combine directory with file name to get the full file path
	filePath := fmt.Sprintf("%s/%s", directory, fileName)

	// Create or open the file for writing
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatal("Error creating file:", err)
	}
	defer file.Close()

	// Write the data to the file
	_, err = file.Write(data)
	if err != nil {
		log.Fatal("Error writing data to file:", err)
	}

	// Confirm successful write
	fmt.Printf("Data successfully saved to '%s'\n", filePath)
}
