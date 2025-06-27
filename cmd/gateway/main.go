// cmd/gateway/main.go
package main

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

	"binance/internal/alerter"
	"binance/internal/handlers"
	"binance/internal/kafka"

	// "binance/internal/metrics"
	"binance/internal/models"

	"github.com/google/uuid"
)

var kafkaBroker = os.Getenv("KAFKA_BROKER")
const (
	consumerTopic = "system_alerts"
)

func main() {
	errs := make(chan error)

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errs <- errors.New((<-c).String())
	}()

	webAlerter := alerter.NewWebAlerter()
	httpSrv := startHTTPServer(webAlerter, errs)

	uniqueConsumerGroup := fmt.Sprintf("gateway-group-%s", uuid.New().String())
	log.Printf("[Gateway] Используется уникальная группа подписчика: %s", uniqueConsumerGroup)

	go runKafkaConsumer(webAlerter, errs, uniqueConsumerGroup)

	// Блокируемся на сигнал
	log.Printf("[Gateway] Приложение запущено. Выход по сигналу: %v", <-errs)

	webAlerter.Shutdown()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Gateway] Ошибка graceful shutdown HTTP сервера: %v", err)
	} else {
		log.Println("[Gateway] HTTP сервер остановлен.")
	}

	log.Println("[Gateway] Приложение завершено.")
}

func runKafkaConsumer(webAlerter *alerter.WebAlerter, errs chan error, consumerGroup string) {
	log.Println("[Gateway] Запуск горутины-моста Kafka->WebAlerter")
	
	// Ожидаем доступности Kafka
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for {
		err := kafka.EnsureTopicExists(ctx, kafkaBroker, consumerTopic)
		if err == nil {
			log.Printf("[Gateway] Топик '%s' готов для работы.", consumerTopic)
			break
		}
		log.Printf("[Gateway] Ожидание Kafka и топика '%s': %v. Повторная попытка через 5 секунд.", consumerTopic, err)
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			errs <- errors.New("контекст отменен, не удалось подключиться к Kafka")
			return
		}
	}


	kafkaReader := kafka.NewReader([]string{kafkaBroker}, consumerGroup, consumerTopic)
	defer kafkaReader.Close()
	log.Println("[Gateway] Kafka reader настроен.")

	logAlerter := alerter.NewLogAlerter()
	compositeAlerter := alerter.NewCompositeAlerter(webAlerter, logAlerter)

	for {
		msg, err := kafkaReader.ReadMessage(context.Background())
		if err != nil {
			log.Printf("[Gateway] Критическая ошибка чтения из Kafka: %v.", err)
			errs <- err // Отправляем ошибку в main и завершаем горутину.
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

func startHTTPServer(webAlerter *alerter.WebAlerter, errs chan error) *http.Server {
	handler := handlers.NewHandler(webAlerter)

	// instrumentedMux := metrics.PrometheusMiddleware(mux)
	srv := newServer(handler)

	log.Printf("[Gateway] Запуск HTTP сервера на порту : %s", srv.Addr)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Gateway] Критическая ошибка HTTP сервера: %v", err)
			errs <- err
		}
	}()
	return srv
}

func newServer(handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         os.Getenv("PORT"),
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}