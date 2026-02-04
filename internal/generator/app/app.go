package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"binance/internal/generator/analysis"
	"binance/internal/pkg/config"
	"binance/internal/pkg/kafka"
	"binance/internal/pkg/models"
)

const (
	consumerTopic = "raw_tickers"
	producerTopic = "system_alerts"
	consumerGroup = "alert-generator-group"
)

type App struct {
	cfg            *config.AppConfig
	kafkaBroker    string
	priceProcessor *analysis.PriceProcessor
}

// New создает новый экземпляр приложения generator.
func New() (*App, error) {
	kafkaBroker := os.Getenv("KAFKA_BROKER")
	if kafkaBroker == "" {
		return nil, fmt.Errorf("адрес Kafka-брокера не может быть пустым")
	}

	cfg := config.Load()

	if err := kafka.EnsureTopicExists(context.Background(), kafkaBroker, producerTopic, 1); err != nil {
		return nil, fmt.Errorf("не удалось создать/проверить топик %s: %w", producerTopic, err)
	}

	kafkaWriter := kafka.NewWriter(producerTopic, kafkaBroker)

	kafkaAlerter := &kafka.KafkaAlerter{Writer: kafkaWriter}
	priceProcessor := analysis.NewPriceProcessor(cfg, kafkaAlerter)

	return &App{
		cfg:            cfg,
		kafkaBroker:    kafkaBroker,
		priceProcessor: priceProcessor,
	}, nil
}

// Run запускает приложение и ожидает сигнала завершения.
func (a *App) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	wg.Add(1)
	go a.runWorker(ctx, &wg)

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)
	<-sigterm

	log.Println("[Generator] Получен сигнал shutdown, завершение работы...")

	cancel()
	wg.Wait()

	if err := a.priceProcessor.Close(); err != nil {
		log.Printf("[Generator] Ошибка при закрытии ресурсов PriceProcessor: %v", err)
	}

	log.Println("[Generator] Приложение корректно завершено.")
}

func (a *App) runWorker(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println("[Generator] Запуск горутины-обработчика...")

	for {
		select {
		case <-ctx.Done():
			log.Println("[Generator] Контекст отменен, горутина завершает работу.")
			return
		default:
		}

		kafkaReader := kafka.NewReader([]string{a.kafkaBroker},
			consumerGroup,
			consumerTopic,
			kafka.WithMinBytes(10e3),
			kafka.WithMaxBytes(10e6),
		)

		log.Println("[Generator] Kafka reader успешно создан, начинаем чтение сообщений.")

		a.processMessages(ctx, &kafkaReader)

		kafkaReader.Reader.Close()

		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (a *App) processMessages(ctx context.Context, reader *kafka.KafkaReader) {
	for {
		msg, err := reader.Reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("[Generator] Сигнал завершения получен во время чтения.")
			} else {
				log.Printf("[Generator] Ошибка чтения из Kafka: %v. Попытка пересоздать reader.", err)
			}
			return
		}

		var ticker models.MiniTicker
		if err := json.Unmarshal(msg.Value, &ticker); err != nil {
			if bytes.HasPrefix(msg.Value, []byte(`{"result":`)) {
				log.Println("[Generator] Получено и проигнорировано сообщение-подтверждение от Binance.")
				continue
			}

			log.Printf("[Generator] Не удалось распарсить сообщение: %s", string(msg.Value))
			continue
		}

		a.priceProcessor.Process([]models.MiniTicker{ticker})
	}
}
