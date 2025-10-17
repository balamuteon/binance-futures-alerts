package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"binance/internal/gateway/alerter"
	"binance/internal/gateway/handlers"
	"binance/internal/pkg/kafka"
	"binance/internal/pkg/metrics"
	"binance/internal/pkg/models"

	"github.com/google/uuid"
)

// App инкапсулирует все компоненты и зависимости сервиса gateway.
type App struct {
	httpServer  *http.Server
	webAlerter  *alerter.WebAlerter
	kafkaBroker string
}

// New создает и конфигурирует новый экземпляр приложения.
func New() (*App, error) {
	kafkaBroker := os.Getenv("KAFKA_BROKER")
	if kafkaBroker == "" {
		log.Fatal("[Gateway] Переменная окружения KAFKA_BROKER не установлена")
	}
	
	webAlerter := alerter.NewWebAlerter()
	handler := handlers.NewHandler(webAlerter)

	instrumentedHandler := metrics.PrometheusMiddleware(handler)
	httpServer := &http.Server{
		Addr:         os.Getenv("PORT"),
		Handler:      instrumentedHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return &App{
		httpServer:  httpServer,
		webAlerter:  webAlerter,
		kafkaBroker: kafkaBroker,
	}, nil
}

// Run запускает все компоненты приложения и блокируется до получения сигнала завершения.
func (a *App) Run() error {
	// Канал для ошибок от горутин
	errs := make(chan error, 1)

	// Запуск HTTP-сервера
	go func() {
		log.Printf("[Gateway] Запуск HTTP сервера на порту: %s", a.httpServer.Addr)
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- fmt.Errorf("ошибка HTTP сервера: %w", err)
		}
	}()

	// Запуск Kafka консюмера
	go a.runKafkaConsumer(errs)

	// Ожидание сигнала завершения
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errs:
		log.Printf("[Gateway] Критическая ошибка от компонента: %v", err)
	case sig := <-quit:
		log.Printf("[Gateway] Получен сигнал завершения: %v", sig)
	}

	return a.shutdown()
}

func (a *App) shutdown() error {
	log.Println("[Gateway] Начало процедуры остановки...")

	a.webAlerter.Shutdown()
	log.Println("[Gateway] WebAlerter остановлен.")

	// Останавливаем HTTP сервер
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Gateway] Ошибка graceful shutdown HTTP сервера: %v", err)
	} else {
		log.Println("[Gateway] HTTP сервер остановлен.")
	}

	log.Println("[Gateway] Приложение завершено.")
	return nil
}

func (a *App) runKafkaConsumer(errs chan<- error) {
	consumerTopic := "system_alerts"
	uniqueConsumerGroup := fmt.Sprintf("gateway-group-%s", uuid.New().String())
	log.Printf("[Gateway] Используется уникальная группа подписчика: %s", uniqueConsumerGroup)

	if err := a.ensureKafkaTopic(consumerTopic); err != nil {
		errs <- err
		return
	}

	kafkaReader := kafka.NewReader([]string{a.kafkaBroker}, uniqueConsumerGroup, consumerTopic)
	defer kafkaReader.Reader.Close()
	log.Println("[Gateway] Kafka reader настроен.")

	logAlerter := alerter.NewLogAlerter()
	compositeAlerter := alerter.NewCompositeAlerter(a.webAlerter, logAlerter)

	for {
		msg, err := kafkaReader.Reader.ReadMessage(context.Background())
		if err != nil {
			errs <- fmt.Errorf("критическая ошибка чтения из Kafka: %w", err)
			return
		}

		var alertMsg models.AlertMessage
		if err := json.Unmarshal(msg.Value, &alertMsg); err != nil {
			log.Printf("[Gateway] Ошибка парсинга JSON алерта: %v", err)
			continue
		}

		compositeAlerter.Alert(
			alertMsg.Symbol,
			alertMsg.PercentageChange,
			alertMsg.CurrentPrice,
			alertMsg.OldestPrice,
		)
	}
}

func (a *App) ensureKafkaTopic(topic string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for {
		err := kafka.EnsureTopicExists(ctx, a.kafkaBroker, topic)
		if err == nil {
			log.Printf("[Gateway] Топик '%s' готов для работы.", topic)
			return nil
		}

		log.Printf("[Gateway] Ожидание Kafka и топика '%s': %v. Повторная попытка через 5 секунд.", topic, err)
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return errors.New("контекст отменен, не удалось подключиться к Kafka")
		}
	}
}
