package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"time"
)

// type of request => 0 -> Request, 1-> Acknowledge, 2-> ok (as in its use is complete)
type Request struct {
	Port string `json:"port"`
	Time int    `json:"time"`
	Type int    `json:"type"`
}
type nodeRequest struct {
	Port string
	Time int
}

var port = ""
var portList = []string{":8001", ":8002", ":8003"}
var logicalClock int = 0
var wantToWrite = false
var ch1 chan int = make(chan int)
var ch2 chan int = make(chan int)
var queue = []nodeRequest{}
var requestQueue = []nodeRequest{}
var portToChanMapper = make(map[string]int)
var enterCriticalSection = make(chan int)

// criticalSection

func write() {
	for {
		permission := 0
		for permission != 7 {
			select {
			case <-ch1:
				permission |= 1
			case <-ch2:
				permission |= (1 << 1)
			case <-enterCriticalSection:
				permission |= (1 << 2)
			}
		}
		file, err := os.OpenFile("critical-section.txt", os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Println("Failed to open file", err)
			return
		}
		defer file.Close()

		_, err = file.WriteString("Enter : " + port + strconv.Itoa(logicalClock) + "\n")
		if err != nil {
			fmt.Println("Failed to write to file: ", err)
		}
		timeTaken := rand.IntN(10)
		time.Sleep(time.Duration(timeTaken) * time.Second)
		logicalClock += timeTaken
		_, err = file.WriteString("Leaving : " + port + strconv.Itoa(logicalClock) + "\n")
		if err != nil {
			fmt.Println("Failed to write to file: ", err)
		}
		wantToWrite = false
		for _, element := range portList {
			if element != port {
				request := Request{
					Time: logicalClock,
					Port: port,
					Type: 2,
				}
				jsonData, _ := json.Marshal(request)
				_, err := http.Post("http://localhost"+element+"/request", "application/json", bytes.NewBuffer(jsonData))
				if err != nil {
					fmt.Println(err)
				}
			}
		}
		floodRequest(1)
	}
}

// Controllers
func handleWrite(w http.ResponseWriter, r *http.Request) {
	wantToWrite = true
	logicalClock++
	queue = append(queue, nodeRequest{Port: port, Time: logicalClock})
	if queue[len(queue)-1].Port == port {
		enterCriticalSection <- 1
	}
	fmt.Println("Flodding request")
	for _, element := range portList {
		if element != port {
			request := Request{
				Time: logicalClock,
				Port: port,
				Type: 0,
			}
			jsonData, _ := json.Marshal(request)
			fmt.Printf("Requesting %s\n", element)
			_, err := http.Post("http://localhost"+element+"/request", "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				fmt.Println(err)
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}
func handleRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var request Request
		json.NewDecoder(r.Body).Decode(&request)
		fmt.Printf("%s %d %d\n", request.Port, request.Type, request.Time)
		logicalClock = max(logicalClock, request.Time)
		logicalClock++
		switch request.Type {
		case 0:
			{
				queue = append(queue, nodeRequest{Port: request.Port, Time: logicalClock})
				if !wantToWrite {
					trequest := Request{
						Time: logicalClock,
						Port: port,
						Type: 1,
					}
					jsonData, _ := json.Marshal(trequest)
					_, err := http.Post("http://localhost"+request.Port+"/request", "application/json", bytes.NewBuffer(jsonData))
					if err != nil {
						fmt.Println(err)
					}
				}
			}
		case 1:
			{
				if portToChanMapper[request.Port] == 0 {
					ch1 <- 1
				} else {
					ch2 <- 1
				}
			}
		case 2:
			{
				queue = remove(queue, nodeRequest{Port: request.Port, Time: request.Time})
				if len(queue) > 0 && queue[len(queue)-1].Port == port {
					enterCriticalSection <- 1

				}
			}
		}
		w.WriteHeader(http.StatusOK)
	}
}

// Helper functiions
func floodRequest(messageType int) {
	logicalClock++
	for _, element := range queue {
		if element.Port == port {
			continue
		}
		request := Request{
			Port: port,
			Time: logicalClock,
			Type: messageType,
		}
		jsonData, _ := json.Marshal(request)
		_, err := http.Post("http://localhost"+element.Port+"/request", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Fatal(err)
		}
	}
	queue = queue[:0]
}

func contains(slice []string, element string) bool {
	for _, x := range slice {
		if x == element {
			return true
		}
	}
	return false
}

func remove(slice []nodeRequest, element nodeRequest) []nodeRequest {
	index := 0
	for i, value := range slice {
		if value.Port != element.Port {
			slice[index] = slice[i]
			index++
		}
	}
	return slice[:index]
}
func main() {
	flag.Parse()
	port = ":" + flag.Arg(0)
	if !contains(portList, port) {
		fmt.Print("The port should be from the following\n")
		fmt.Print(portList)
	}
	go write()
	var count int = 0
	for _, element := range portList {
		if element != port {
			portToChanMapper[element] = count
			count++
		}
	}
	http.HandleFunc("/write", handleWrite)
	http.HandleFunc("/request", handleRequests)
	fmt.Printf("Starting a port at %s", port)
	http.ListenAndServe(port, nil)
}
