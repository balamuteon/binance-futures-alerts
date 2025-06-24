// cmd/gateway/main.go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"binance/internal/alerter"
	"binance/internal/kafka"

	// "binance/internal/metrics"
	"binance/internal/models"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	kafkaGo "github.com/segmentio/kafka-go"
)

const (
	consumerTopic = "system_alerts"
	consumerGroup = "gateway-group"
)

func main() {
	// 1. Создаем канал для ошибок, который будет блокировать main.
	errs := make(chan error)

	// 2. Запускаем обработчик сигналов ОС, который отправит сигнал в наш канал.
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errs <- errors.New((<-c).String()) // Отправляем полученный сигнал как ошибку.
	}()

	// --- Запуск компонентов приложения ---
	webAlerter := alerter.NewWebAlerter()
	httpSrv := startHTTPServer(webAlerter, errs) // Передаем канал ошибок в http сервер.

	// Запускаем потребителя Kafka в отдельной горутине.
	go runKafkaConsumer(webAlerter, errs)

	// 3. Блокируем main горутину, ожидая первую ошибку или сигнал завершения.
	log.Printf("[Gateway] Приложение запущено. Выход по сигналу: %v", <-errs)

	// --- Корректное завершение ---
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

func runKafkaConsumer(webAlerter *alerter.WebAlerter, errs chan error) {
	log.Println("[Gateway] Запуск горутины-моста Kafka->WebAlerter")
	
	// Ожидаем доступности Kafka
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for {
		err := kafka.EnsureTopicExists(ctx, consumerTopic)
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


	kafkaReader := kafkaGo.NewReader(kafkaGo.ReaderConfig{
		Brokers: []string{kafka.KafkaBroker},
		GroupID: consumerGroup,
		Topic:   consumerTopic,
	})
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

// Функции startHTTPServer и newServer остаются без изменений
func startHTTPServer(webAlerter *alerter.WebAlerter, errs chan error) *http.Server {
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/", fs)
	mux.Handle("/ws/alerts", webAlerter)
	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// instrumentedMux := metrics.PrometheusMiddleware(mux)
	srv := newServer(mux)

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