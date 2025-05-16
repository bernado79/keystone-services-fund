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
	"strings"
	"time"

	"cloud.google.com/go/logging"
	"github.com/gorilla/mux"
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

type IndexData struct {
	Date     string  `json:"date"`
	AdjClose float64 `json:"adjusted_close"`
}

func (a *App) Handler(w http.ResponseWriter, r *http.Request) {
	a.log.Log(logging.Entry{
		Severity: logging.Info,
		HTTPRequest: &logging.HTTPRequest{
			Request: r,
		},
		Payload: "Request received",
	})

	// get the /{symbol} from the URL
	vars := mux.Vars(r)
	symbol := vars["symbol"]

	ratioVOO := 9
	ratioBTC := 1

	// Check if the symbol is not provided
	if symbol == "" {
		http.Error(w, "Symbol is required", http.StatusBadRequest)
		return
	}

	// Case insensitive check for the symbol
	symbol = strings.ToUpper(symbol)

	// Switch case to handle different symbols
	switch symbol {
	case "QUARTZ9":
		ratioVOO = 9
		ratioBTC = 1
	case "QUARTZ7":
		ratioVOO = 7
		ratioBTC = 3
	case "QUARTZ5":
		ratioVOO = 5
		ratioBTC = 5
	default:
		http.Error(w, "Invalid symbol", http.StatusBadRequest)
		return
	}

	// Get the symbol from the URL query parameters
	stockDataVOO, err := a.PrepareSymbolJSONData("VOO.US", "2019-01-02")
	if err != nil {
		log.Fatal("Error preparing symbol JSON data:", err)
	}

	stockDataBTC, err := a.PrepareSymbolJSONData("BTC-USD.CC", "2019-01-02")
	if err != nil {
		log.Fatal("Error preparing symbol JSON data:", err)
	}

	stockDataVOOFF := forwardFillStockData(stockDataVOO, "2019-01-02", stockDataBTC[len(stockDataBTC)-1].Date)

	// Create a map to store the stock data by date
	stockDataVOOMap := make(map[string]StockData)
	for _, data := range stockDataVOOFF {
		stockDataVOOMap[data.Date] = data
	}

	stockDataIndex := make([]IndexData, 0)

	// Calculate index at the start
	stockDataIndex = append(stockDataIndex, IndexData{
		Date:     stockDataBTC[0].Date,
		AdjClose: 100,
	})
	initialIndexValue := (stockDataBTC[0].AdjClose * float64(ratioBTC)) + (stockDataVOOFF[0].AdjClose * float64(ratioVOO))

	for i := 1; i < len(stockDataBTC); i++ {
		currentIndexValue := (stockDataBTC[i].AdjClose * float64(ratioBTC)) + (stockDataVOOFF[i].AdjClose * float64(ratioVOO))
		indexValue := (currentIndexValue / initialIndexValue) * 100
		stockDataIndex = append(stockDataIndex, IndexData{
			Date:     stockDataBTC[i].Date,
			AdjClose: indexValue,
		})
	}

	// Return stockDataIndex as JSON
	jsonIndexData, err := json.Marshal(stockDataIndex)
	if err != nil {
		log.Fatal("Error marshalling JSON data:", err)
	}

	// set the content type to JSON
	w.Header().Set("Content-Type", "application/json")

	// Allow for cross-origin requests from any origin
	w.Header().Set("Access-Control-Allow-Origin", "*")

	fmt.Fprintf(w, "%s", jsonIndexData)
}

func (a *App) PrepareSymbolJSONData(symbol string, startDate string) ([]StockData, error) {
	// URL of the EOD Historical API (replace with the actual endpoint)
	url := "https://eodhd.com/api/eod/" + symbol + "?api_token=" + a.EODAPIKEY + "&fmt=json&from=" + startDate

	currentUTCDate := time.Now().UTC().Format(time.DateOnly)
	directory := a.bucketCacheDirectory + "/" + symbol
	fileName := currentUTCDate + ".json"
	fullPath := a.bucketCacheDirectory + "/" + symbol + "/" + currentUTCDate + ".json"

	// Check if the file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		// If the file does not exist, read data from the URL
		body, err := readDataFromURL(url)
		if err != nil {
			log.Fatal("Error reading data from URL:", err)
		}
		// Parse the JSON data into a slice of StockData
		var stockData []StockData
		err = json.Unmarshal(body, &stockData)
		if err != nil {
			log.Fatal("Error unmarshalling JSON data:", err)
		}

		// Save the data to a file
		saveData(body, directory, fileName)
		if err != nil {
			log.Fatal("Error saving data to file:", err)
		}

		// Confirm successful write
		fmt.Printf("Data successfully saved to '%s'\n", fullPath)
	}

	// If the file exists, read data from the file
	fileData, err := os.ReadFile(fullPath)
	if err != nil {
		log.Fatal("Error reading data from file:", err)
	}

	// Parse the JSON data into a slice of StockData
	var stockData []StockData
	err = json.Unmarshal(fileData, &stockData)
	if err != nil {
		log.Fatal("Error unmarshalling JSON data from file:", err)
	}

	// Return the JSON data
	return stockData, nil
}

// Function to forward fill the StockData slice for missing inbetween dates from the start date to the end date。 FF based on the last available data from the previous date
func forwardFillStockData(stockData []StockData, startDate string, endDate string) []StockData {
	// Create a map to store the stock data by date
	stockDataMap := make(map[string]StockData)
	for _, data := range stockData {
		stockDataMap[data.Date] = data
	}

	// Create a slice to hold the forward-filled data
	var filledData []StockData

	// Iterate through the date range and fill in missing dates
	currentDate := startDate
	for currentDate <= endDate {
		if data, exists := stockDataMap[currentDate]; exists {
			filledData = append(filledData, data)
		} else {
			// If the date does not exist, use the last available data
			if len(filledData) > 0 {
				lastData := filledData[len(filledData)-1]
				data := StockData{
					Date:     currentDate,
					Open:     lastData.Open,
					High:     lastData.High,
					Low:      lastData.Low,
					Close:    lastData.Close,
					AdjClose: lastData.AdjClose,
					Volume:   lastData.Volume,
				}
				filledData = append(filledData, data)
			}
		}
		currentDate = incrementDate(currentDate)
	}

	return filledData
}

// Function to read data from URL and return body
func readDataFromURL(url string) ([]byte, error) {
	// Send a GET request to the URL
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the body of the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// saveData saves the JSON data to a file in a specific directory
// filename optional, if not provided, a default name will be used
func saveData(data []byte, fileDirectory string, fileName string) {
	// Ensure the directory exists, create it if it doesn't
	if _, err := os.Stat(fileDirectory); os.IsNotExist(err) {
		err := os.MkdirAll(fileDirectory, os.ModePerm)
		if err != nil {
			log.Fatal("Error creating directory:", err)
		}
	}

	// Combine directory with file name to get the full file path
	filePath := fmt.Sprintf("%s/%s", fileDirectory, fileName)

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

// incrementDate increments a date string by one day.
func incrementDate(date string) string {
	t, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return "" // Handle error appropriately in real application
	}
	t = t.AddDate(0, 0, 1)
	return t.Format(time.DateOnly)
}
