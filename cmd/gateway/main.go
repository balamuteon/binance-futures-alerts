// cmd/gateway/main.go
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"binance/internal/alerter"
	"binance/internal/kafka" // <-- Используем наш общий пакет
	"binance/internal/metrics"
	"binance/internal/models"
	kafkaGo "github.com/segmentio/kafka-go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	consumerTopic = "system_alerts"
	consumerGroup = "gateway-group"
	configUpdateTopic = "config_updates"
)

func main() {
	kafkaBroker := os.Getenv("KAFKA_BROKER")

	webAlerter := alerter.NewWebAlerter()
	logAlerter := alerter.NewLogAlerter()
	compositeAlerter := alerter.NewCompositeAlerter(webAlerter, logAlerter)
	httpSrv := startHTTPServer(webAlerter)

	// --- Ожидаем доступности Kafka ---
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
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
			log.Fatalf("[Gateway] Контекст отменен, не удалось подключиться к Kafka.")
		}
	}

	kafkaReader := kafkaGo.NewReader(kafkaGo.ReaderConfig{
		Brokers: []string{kafkaBroker},
		GroupID: consumerGroup,
		Topic:   consumerTopic,
		
	})
	defer kafkaReader.Close()
	log.Println("[Gateway] Kafka reader настроен.")

	go func() {
		log.Println("[Gateway] Запуск горутины-моста Kafka->WebAlerter")
		for {
			msg, err := kafkaReader.ReadMessage(context.Background())
			if err != nil {
				log.Printf("[Gateway] Ошибка чтения из Kafka: %v", err)
				break
			}
			log.Printf("[Gateway] Получено сообщение из Kafka: %s", string(msg.Value))

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
	}()

	// --- Graceful Shutdown ---
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Println("[Gateway] Приложение запущено. Для выхода нажмите Ctrl+C.")
	<-interrupt
	log.Println("[Gateway] Получен сигнал прерывания, начинаем graceful shutdown...")

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

// Функции startHTTPServer и newServer остаются без изменений
func startHTTPServer(webAlerter *alerter.WebAlerter) *http.Server {
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/", fs)
	mux.Handle("/ws/alerts", webAlerter)
	mux.Handle("/metrics", promhttp.Handler())

	instrumentedMux := metrics.PrometheusMiddleware(mux)
	srv := newServer(instrumentedMux)

	log.Printf("[Gateway] Запуск HTTP сервера на порту : %s", os.Getenv("PORT"))
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Gateway] Критическая ошибка HTTP сервера: %v", err)
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