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

	"binance/internal/pkg/config"
	"binance/internal/pkg/kafka"
	"binance/internal/pkg/models"
	websocketclient "binance/internal/pkg/webSocketClient"

	kafkaGO "github.com/segmentio/kafka-go"
)

const (
	topic                = "raw_tickers"
	rawTickersPartitions = 12
)

// App инкапсулирует всю логику сервиса ingestor.
type App struct {
	cfg         *config.AppConfig
	kafkaBroker string
	kafkaWriter *kafkaGO.Writer
}

// New создает и конфигурирует новый экземпляр приложения ingestor.
func New() (*App, error) {
	kafkaBroker := os.Getenv("KAFKA_BROKER")
	if kafkaBroker == "" {
		return nil, fmt.Errorf("адрес Kafka-брокера не может быть пустым")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := ensureKafkaTopic(ctx, kafkaBroker, topic, rawTickersPartitions); err != nil {
		return nil, err
	}

	return &App{
		cfg:         config.Load(),
		kafkaBroker: kafkaBroker,
		kafkaWriter: kafka.NewWriter(topic, kafkaBroker),
	}, nil
}

// Run запускает приложение и блокируется до получения сигнала о завершении.
func (a *App) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(1)
	go a.runConnectionManager(ctx, &wg)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Ingestor] Получен сигнал прерывания, начинаем graceful shutdown...")

	cancel()
	wg.Wait()

	if err := a.kafkaWriter.Close(); err != nil {
		log.Printf("[Ingestor] Ошибка при закрытии Kafka writer: %v", err)
	}

	log.Println("[Ingestor] Приложение корректно завершено.")
}

func (a *App) runConnectionManager(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			log.Println("[Ingestor] Контекст отменен, менеджер подключений завершает работу.")
			return
		default:
		}

		client, err := websocketclient.New(a.cfg.FuturesWebSocketURL)
		if err != nil {
			log.Printf("[Ingestor] Ошибка подключения к Binance: %v. Повторная попытка через 5с.", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		client.Subscribe(a.cfg.StreamName, 1)
		messagesChan, errsChan := client.ReadMessages(ctx)
		log.Println("[Ingestor] Успешно подключились и подписались. Начинаем читать сообщения от Binance...")

		err = a.processMessages(ctx, messagesChan, errsChan)
		if err != nil {
			log.Printf("[Ingestor] Сессия прервана из-за ошибки: %v", err)
		}

		client.Close()
		log.Println("[Ingestor] Сессия завершена. Переподключение через 5 секунд...")

		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (a *App) processMessages(ctx context.Context, messages <-chan []byte, errs <-chan error) error {
	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				return fmt.Errorf("канал сообщений был закрыт")
			}

			var tickers []models.MiniTicker
			if err := json.Unmarshal(msg, &tickers); err != nil {
				if bytes.HasPrefix(msg, []byte(`{"result":`)) {
					log.Println("[Ingestor] Получено и проигнорировано сообщение-подтверждение от Binance.")
					continue
				}
				log.Printf("[Ingestor] Не удалось распарсить сообщение: %s", string(msg))
				continue
			}

			if len(tickers) == 0 {
				continue
			}

			kafkaMessages := make([]kafkaGO.Message, 0, len(tickers))
			for _, ticker := range tickers {
				payload, err := json.Marshal(ticker)
				if err != nil {
					log.Printf("[Ingestor] Ошибка кодирования тикера %s в JSON: %v", ticker.Symbol, err)
					continue
				}
				kafkaMessages = append(kafkaMessages, kafkaGO.Message{
					Key:   []byte(ticker.Symbol),
					Value: payload,
				})
			}

			if len(kafkaMessages) == 0 {
				continue
			}

			err := a.kafkaWriter.WriteMessages(ctx, kafkaMessages...)
			if err != nil {
				return fmt.Errorf("ошибка записи в Kafka: %w", err)
			}

		case err, ok := <-errs:
			if !ok {
				return fmt.Errorf("канал ошибок был закрыт")
			}
			return fmt.Errorf("ошибка от WebSocket клиента: %w", err)

		case <-ctx.Done():
			log.Println("[Ingestor] Получен сигнал shutdown во время обработки сообщений.")
			return ctx.Err()
		}
	}
}

func ensureKafkaTopic(ctx context.Context, kafkaBroker, topic string, partitions int) error {
	for {
		err := kafka.EnsureTopicExists(ctx, kafkaBroker, topic, partitions)
		if err == nil {
			log.Printf("[Ingestor] Топик Kafka '%s' готов.", topic)
			return nil
		}
		log.Printf("[Ingestor] Не удалось настроить топик Kafka: %v. Повторная попытка через 5 секунд.", err)

		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return fmt.Errorf("контекст отменен, не удалось настроить Kafka: %w", ctx.Err())
		}
	}
}
