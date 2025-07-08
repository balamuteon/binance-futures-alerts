package main

import (
	"log"
	"os"

	"binance/internal/gateway/app"
)

func main() {
	kafkaBroker := os.Getenv("KAFKA_BROKER")
	if kafkaBroker == "" {
		log.Fatal("[Gateway] Переменная окружения KAFKA_BROKER не установлена")
	}

	application, err := app.New(kafkaBroker)
	if err != nil {
		log.Fatalf("[Gateway] Не удалось создать приложение: %v", err)
	}

	if err := application.Run(); err != nil {
		log.Fatalf("[Gateway] Критическая ошибка во время работы приложения: %v", err)
	}
}