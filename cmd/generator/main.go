package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"binance/internal/analysis"
	"binance/internal/config"
	"binance/internal/kafka"
	"binance/internal/models"

)

var kafkaBroker = os.Getenv("KAFKA_BROKER")
const (
	consumerTopic = "raw_tickers"
	producerTopic = "system_alerts"
	consumerGroup = "alert-generator-group"
)

func main() {
	cfg := config.Load()

	if err := kafka.EnsureTopicExists(context.Background(), kafkaBroker, producerTopic); err != nil {
		log.Fatalf("[Generator] Не удалось создать/проверить топик %s: %v", producerTopic, err)
	}

	kafkaWriter := kafka.NewWriter(producerTopic, kafkaBroker)
	defer kafkaWriter.Close()
	log.Println("[Generator] Kafka writer настроен.")

	kafkaAlerter := &kafka.KafkaAlerter{Writer: kafkaWriter}
	priceProcessor := analysis.NewPriceProcessor(cfg, kafkaAlerter)
	log.Println("[Generator] PriceProcessor настроен.")

	// --- Graceful Shutdown ---
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// --- Запуск горутины-обработчика с логикой переподключения ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("[Generator] Запуск горутины-обработчика...")

		for {
			select {
			case <-ctx.Done():
				log.Println("[Generator] Контекст отменен, горутина завершает работу.")
				return
			default:
			}

			kafkaReader := kafka.NewReader([]string{kafkaBroker},
				consumerGroup,
				consumerTopic,
				kafka.WithMinBytes(10e3),
				kafka.WithMaxBytes(10e6),
			)
			log.Println("[Generator] Kafka reader успешно создан, начинаем чтение сообщений.")

			for {
				msg, err := kafkaReader.ReadMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						log.Println("[Generator] Сигнал завершения получен во время чтения.")
						break
					}
					log.Printf("[Generator] Ошибка чтения из Kafka: %v. Попытка пересоздать reader через 5 секунд.", err)
					break
				}

				var tickers []models.MiniTicker
				if err := json.Unmarshal(msg.Value, &tickers); err != nil {
					if bytes.HasPrefix(msg.Value, []byte(`{"result":`)) {
						log.Println("[Ingestor] Получено и проигнорировано сообщение-подтверждение от Binance.")
						continue
					}
					log.Printf("[Generator] Не удалось распарсить сообщение: %s", string(msg.Value))
					continue
				}

				if len(tickers) > 0 {
					priceProcessor.Process(tickers)
				}
			}

			kafkaReader.Close()

			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
			}
		}
	}()

	<-sigterm
	log.Println("[Generator] Получен сигнал shutdown, завершение работы...")
	cancel()
	wg.Wait()

	log.Println("[Generator] Приложение корректно завершено.")
}
