package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"binance/internal/pkg/config"
	"binance/internal/pkg/kafka"
	websocketclient "binance/internal/pkg/webSocketClient"

	kafkaGO "github.com/segmentio/kafka-go"
)

const topic = "raw_tickers"

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

	// Ожидание готовности Kafka с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := ensureKafkaTopic(ctx, kafkaBroker, topic); err != nil {
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

	// Запускаем нашего воркера, который будет подключаться к Binance
	wg.Add(1)
	go a.runConnectionManager(ctx, &wg)

	// Ожидаем сигнала прерывания (Ctrl+C)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Ingestor] Получен сигнал прерывания, начинаем graceful shutdown...")

	// Даем команду на завершение всем горутинам
	cancel()
	// И ждем их полной остановки
	wg.Wait()

	// Корректно закрываем Kafka writer после того, как все горутины завершились
	if err := a.kafkaWriter.Close(); err != nil {
		log.Printf("[Ingestor] Ошибка при закрытии Kafka writer: %v", err)
	}

	log.Println("[Ingestor] Приложение корректно завершено.")
}

// runConnectionManager управляет жизненным циклом подключения к WebSocket Binance.
func (a *App) runConnectionManager(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	// Бесконечный цикл для автоматического переподключения
	for {
		// Перед каждой новой попыткой проверяем, не пора ли завершать работу.
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

		// Запускаем внутренний цикл обработки сообщений для текущей сессии
		err = a.processMessages(ctx, messagesChan, errsChan)
		if err != nil {
			log.Printf("[Ingestor] Сессия прервана из-за ошибки: %v", err)
		}

		client.Close()
		log.Println("[Ingestor] Сессия завершена. Переподключение через 5 секунд...")

		// Пауза перед переподключением с возможностью немедленного выхода
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// processMessages читает из каналов сообщений и ошибок и отправляет данные в Kafka.
func (a *App) processMessages(ctx context.Context, messages <-chan []byte, errs <-chan error) error {
	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				return fmt.Errorf("канал сообщений был закрыт")
			}

			// Передаем контекст в WriteMessages, чтобы он тоже мог быть прерван
			err := a.kafkaWriter.WriteMessages(ctx, kafkaGO.Message{Value: msg})
			if err != nil {
				// Если запись в Kafka не удалась, это критично. Разрываем сессию.
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

// ensureKafkaTopic - вспомогательная функция для ожидания готовности топика.
func ensureKafkaTopic(ctx context.Context, kafkaBroker, topic string) error {
	for {
		err := kafka.EnsureTopicExists(ctx, kafkaBroker, topic)
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
