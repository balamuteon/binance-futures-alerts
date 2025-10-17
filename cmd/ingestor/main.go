package main

import (
	"log"
	"binance/internal/ingestor/app"
)

func main() {
	application, err := app.New()
	if err != nil {
		log.Fatalf("[Ingestor] Не удалось создать приложение: %v", err)
	}

	application.Run()
}
