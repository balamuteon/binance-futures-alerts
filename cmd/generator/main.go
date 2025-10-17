package main

import (
	"log"

	"binance/internal/generator/app"
)

func main() {
	application, err := app.New()
	if err != nil {
		log.Fatalf("[Generator] Не удалось создать приложение: %v", err)
	}

	application.Run()
}
