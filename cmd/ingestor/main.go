package main

import (
	"log"
	"os"

	"binance/internal/ingestor/app"
)

func main() {
	kafkaBroker := os.Getenv("KAFKA_BROKER")

	application, err := app.New(kafkaBroker)
	if err != nil {
		log.Fatalf("[Ingestor] Не удалось создать приложение: %v", err)
	}

	application.Run()
}