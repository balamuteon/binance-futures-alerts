package main

import (
	"log"

	"binance/internal/gateway/app"
)

func main() {
	application, err := app.New()
	if err != nil {
		log.Fatalf("[Gateway] Не удалось создать приложение: %v", err)
	}

	if err := application.Run(); err != nil {
		log.Fatalf("[Gateway] Критическая ошибка во время работы приложения: %v", err)
	}
}