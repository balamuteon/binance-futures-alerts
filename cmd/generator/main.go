package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"

	"binance/internal/analysis"
	"binance/internal/config"
	"binance/internal/kafka"
	"binance/internal/models"

	kafkaGO "github.com/segmentio/kafka-go"
)

const (
	consumerTopic = "raw_tickers"
	producerTopic = "system_alerts"
	consumerGroup = "alert-generator-group" // Группа потребителей
)

// KafkaAlerter - это специальный "адаптер", который реализует интерфейс Alerter.
// Вместо отправки в WebSocket, он отправляет алерты в Kafka.

func main() {
	cfg := config.Load()

	kafkaBroker := os.Getenv("KAFKA_BROKER")

	if err := kafka.EnsureTopicExists(context.Background(), kafkaBroker, producerTopic); err != nil {
		log.Fatalf("[Generator] Не удалось создать/проверить топик %s: %v", producerTopic, err)
	}

	// --- Настройка Kafka Consumer (Reader) ---
	kafkaReader := kafkaGO.NewReader(kafkaGO.ReaderConfig{
		Brokers:  []string{kafkaBroker},
		GroupID:  consumerGroup,
		Topic:    consumerTopic,
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
	})
	defer kafkaReader.Close()
	log.Println("[Generator] Kafka reader настроен.")

	// --- Настройка Kafka Producer (Writer) ---
	kafkaWriter := &kafkaGO.Writer{
		Addr:     kafkaGO.TCP(kafkaBroker),
		Topic:    producerTopic,
		Balancer: &kafkaGO.LeastBytes{},
	}
	defer kafkaWriter.Close()
	log.Println("[Generator] Kafka writer настроен.")

	// --- Настройка логики анализа ---
	kafkaAlerter := &kafka.KafkaAlerter{Writer: kafkaWriter}
	priceProcessor := analysis.NewPriceProcessor(cfg, kafkaAlerter)
	log.Println("[Generator] PriceProcessor настроен.")

	// --- Graceful Shutdown ---
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	// --- Главный цикл обработки ---
	log.Println("[Generator] Запуск главного цикла обработки сообщений из Kafka...")
	for {
		select {
		case <-sigterm:
			log.Println("[Generator] Получен сигнал shutdown, завершение работы.")
			return
		default:
			// Читаем следующее сообщение из топика raw_tickers
			msg, err := kafkaReader.ReadMessage(context.Background())
			if err != nil {
				log.Printf("[Generator] Ошибка чтения из Kafka: %v", err)
				break
			}

			var tickers []models.MiniTicker
			// Binance отправляет данные как массив, поэтому парсим как массив
			if err := json.Unmarshal(msg.Value, &tickers); err != nil {
				// log.Printf("[Generator] Не удалось распарсить сообщение: %s", string(msg.Value))
				continue
			}

			// Если удалось распарсить, отправляем на обработку
			if len(tickers) > 0 {
				priceProcessor.Process(tickers)
			}
		}
	}
}
