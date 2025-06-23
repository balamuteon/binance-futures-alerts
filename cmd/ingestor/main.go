package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"time"

	"binance/internal/config"
	"binance/internal/kafka"
	websocketclient "binance/internal/webSocketClient"

	kafkaGO "github.com/segmentio/kafka-go"
)

const topic = "raw_tickers"

func main() {
	cfg := config.Load()
	kafkaBroker := os.Getenv("KAFKA_BROKER")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	for {
		err := kafka.EnsureTopicExists(ctx, kafkaBroker, topic)
		if err == nil {
			break
		}
		log.Printf("[Ingestor] Не удалось настроить топик Kafka: %v. Повторная попытка через 5 секунд.", err)
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			log.Fatalf("[Ingestor] Контекст отменен, не удалось настроить Kafka.")
		}
	}

	// --- Настройка Kafka Producer ---
	kafkaWriter := &kafkaGO.Writer{
		Addr:     kafkaGO.TCP(kafkaBroker),
		Topic:    topic,
		Balancer: &kafkaGO.LeastBytes{},
	}
	defer kafkaWriter.Close()
	log.Println("[Ingestor] Kafka writer настроен.")

	// --- Основная логика ---
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	appShutdownSignal := make(chan struct{})

	// Запускаем менеджер подключений, который теперь будет писать в Kafka
	go runBinanceConnectionManager(cfg, kafkaWriter, appShutdownSignal)

	// Ждем сигнала Ctrl+C
	<-interrupt
	log.Println("[Ingestor] Получен сигнал прерывания, начинаем graceful shutdown...")
	close(appShutdownSignal)

	log.Println("[Ingestor] Приложение завершено.")
}

// Эта функция теперь принимает kafkaGO.Writer вместо PriceProcessor
func runBinanceConnectionManager(cfg *config.AppConfig, kafkaWriter *kafkaGO.Writer, appShutdownSignal chan struct{}) {
	for {
		client, err := websocketclient.New(cfg.FuturesWebSocketURL)
		if err != nil {
			log.Printf("[Ingestor] Ошибка подключения к Binance: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		client.Subscribe(cfg.StreamName, 1)
		messagesChan, errsChan := client.ReadMessages(appShutdownSignal)
		log.Println("[Ingestor] Начинаем читать сообщения от Binance...")

	readerLoop:
		for {
			select {
			case msg, ok := <-messagesChan:
				if !ok {
					log.Println("[Ingestor] Канал сообщений закрыт.")
					break readerLoop
				}
				err := kafkaWriter.WriteMessages(context.Background(),
					kafkaGO.Message{
						Value: msg,
					},
				)
				if err != nil {
					log.Printf("[Ingestor] Ошибка записи в Kafka: %v", err)
				}

			case err, ok := <-errsChan:
				if !ok {
					log.Println("[Ingestor] Канал ошибок закрыт.")
					break readerLoop
				}
				log.Printf("[Ingestor] Ошибка от WebSocket клиента: %v", err)
				break readerLoop

			case <-appShutdownSignal:
				log.Println("[Ingestor] Получен сигнал shutdown.")
				break readerLoop
			}
		}

		client.Close()
		log.Println("[Ingestor] Сессия завершена, переподключение через 5 секунд...")

		select {
		case <-appShutdownSignal:
			return
		case <-time.After(5 * time.Second):
		}
	}
}
