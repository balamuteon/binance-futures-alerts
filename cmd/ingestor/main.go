package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync" // <-- Импортируем sync
	"syscall"
	"time"

	"binance/internal/config"
	"binance/internal/kafka"
	websocketclient "binance/internal/webSocketClient"

	kafkaGO "github.com/segmentio/kafka-go"
)

const topic = "raw_tickers"

func main() {
	cfg := config.Load()

	// Таймаут на проверку топика
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Ожидание готовности Kafka
	for {
		err := kafka.EnsureTopicExists(ctx, topic)
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

	kafkaWriter := &kafkaGO.Writer{
		Addr:     kafkaGO.TCP(kafka.KafkaBroker),
		Topic:    topic,
		Balancer: &kafkaGO.LeastBytes{},
	}
	defer kafkaWriter.Close()
	log.Println("[Ingestor] Kafka writer настроен.")

	// --- Улучшенный Graceful Shutdown ---
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// Запускаем менеджер подключений
	wg.Add(1)
	go runBinanceConnectionManager(shutdownCtx, &wg, cfg, kafkaWriter)

	// Ожидаем сигнала прерывания (Ctrl+C)
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interrupt

	log.Println("[Ingestor] Получен сигнал прерывания, начинаем graceful shutdown...")
	shutdownCancel() // Сигнализируем горутине о необходимости завершения
	wg.Wait()          // Ждем, пока горутина действительно завершится

	log.Println("[Ingestor] Приложение корректно завершено.")
}

func runBinanceConnectionManager(ctx context.Context, wg *sync.WaitGroup, cfg *config.AppConfig, kafkaWriter *kafkaGO.Writer) {
	defer wg.Done()

	// Цикл для переподключений
	for {
		// Проверяем, не пора ли завершать работу, перед новой попыткой.
		select {
		case <-ctx.Done():
			log.Println("[Ingestor] Контекст отменен, менеджер подключений завершает работу.")
			return
		default:
		}

		client, err := websocketclient.New(cfg.FuturesWebSocketURL)
		if err != nil {
			log.Printf("[Ingestor] Ошибка подключения к Binance: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		client.Subscribe(cfg.StreamName, 1)
		messagesChan, errsChan := client.ReadMessages(ctx) // Передаем контекст в ридер
		log.Println("[Ingestor] Начинаем читать сообщения от Binance...")

	readerLoop:
		for {
			select {
			case msg, ok := <-messagesChan:
				if !ok {
					log.Println("[Ingestor] Канал сообщений закрыт.")
					break readerLoop
				}
				// Передаем контекст в WriteMessages, чтобы он тоже мог быть прерван
				err := kafkaWriter.WriteMessages(ctx, kafkaGO.Message{Value: msg})
				if err != nil {
					// Если запись в Kafka не удалась, разрываем цикл, чтобы переподключиться.
					log.Printf("[Ingestor] Ошибка записи в Kafka: %v. Разрываем сессию для переподключения.", err)
					break readerLoop
				}

			case err, ok := <-errsChan:
				if !ok {
					log.Println("[Ingestor] Канал ошибок закрыт.")
					break readerLoop
				}
				log.Printf("[Ingestor] Ошибка от WebSocket клиента: %v", err)
				break readerLoop

			case <-ctx.Done():
				log.Println("[Ingestor] Получен сигнал shutdown.")
				break readerLoop
			}
		}

		client.Close()
		log.Println("[Ingestor] Сессия завершена, переподключение через 5 секунд...")

		// Пауза перед переподключением, с возможностью немедленного выхода при shutdown.
		select {
		case <-ctx.Done():
			// Выходим из основного цикла for, если shutdown пришел во время паузы
			return
		case <-time.After(5 * time.Second):
		}
	}
}