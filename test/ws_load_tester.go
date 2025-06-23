// ws_receiver_tester.go
package main

import (
	"flag"
	"log"
	"net/url"
	// "os"
	// "os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var (
	successfulConns uint64
	failedConns     uint64
	messageCount    uint64
)

func main() {
	addr := flag.String("addr", "localhost:8080", "websocket service address")
	path := flag.String("path", "/ws", "websocket path")
	numClients := flag.Int("c", 100, "number of concurrent clients")
	testDuration := flag.Duration("d", 30*time.Second, "duration of the test")
	flag.Parse()

	log.Printf("Starting WebSocket receive test on %s%s", *addr, *path)
	log.Printf("Clients: %d, Duration: %s", *numClients, *testDuration)

	u := url.URL{Scheme: "ws", Host: *addr, Path: *path}
	
	var wg sync.WaitGroup
	for i := 0; i < *numClients; i++ {
		wg.Add(1)
		go runClient(u.String(), &wg)
	}

	// Ожидаем указанное время
	time.Sleep(*testDuration)
	log.Println("Test duration finished.")

	log.Println("\n--- Test Results ---")
	printResults(*testDuration)
}

func runClient(url string, wg *sync.WaitGroup) {
	defer wg.Done()

	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		atomic.AddUint64(&failedConns, 1)
		return
	}
	defer c.Close()
	atomic.AddUint64(&successfulConns, 1)

	// Просто читаем сообщения в цикле, пока соединение не закроется
	for {
		_, _, err := c.ReadMessage()
		if err != nil {
			// Ошибка означает, что сервер закрыл соединение или произошел сбой
			break
		}
		// Увеличиваем счетчик полученных сообщений
		atomic.AddUint64(&messageCount, 1)
	}
}

func printResults(duration time.Duration) {
	sConns := atomic.LoadUint64(&successfulConns)
	fConns := atomic.LoadUint64(&failedConns)
	totalMsgs := atomic.LoadUint64(&messageCount)

	log.Printf("Successful connections: %d", sConns)
	log.Printf("Failed connections: %d", fConns)
	log.Printf("Total messages received: %d", totalMsgs)

	if duration.Seconds() > 0 && totalMsgs > 0 {
		msgsPerSec := float64(totalMsgs) / duration.Seconds()
		log.Printf("Average throughput: %.2f messages/sec", msgsPerSec)
	}
}