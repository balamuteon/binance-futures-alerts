package main

import (
	"log"
	"os"

	"binance/internal/generator/app"
)

func main() {
	kafkaBroker := os.Getenv("KAFKA_BROKER")
	
	application, err := app.New(kafkaBroker)
	if err != nil {
		log.Fatalf("[Generator] Не удалось создать приложение: %v", err)
	}

	application.Run()
}