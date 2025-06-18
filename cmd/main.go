package main

import (
	"binance/internal/alerter"
	"binance/internal/analysis"
	"binance/internal/config"
	"binance/internal/models"
	websocketclient "binance/internal/webSocketClient"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"
)

func main() {
	cfg := config.Load()
	wg, priceProcessor, webAlerter, interruptChan := setupApplication(cfg)

	// Канал для сигнализации менеджеру соединений Binance о полном завершении приложения
	appShutdownSignalForBinanceMgr := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		runBinanceConnectionManager(cfg, priceProcessor, appShutdownSignalForBinanceMgr)
	}()

	httpSrv := startHTTPServer(webAlerter)

	log.Println("Приложение запущено. Для выхода нажмите Ctrl+C.")

	// --- Начало Graceful Shutdown ---
	<-interruptChan
	log.Println("Получен сигнал прерывания, начинаем graceful shutdown...")

	// 1. Сигнализируем менеджеру соединений Binance о необходимости завершения
	log.Println("Отправка сигнала завершения менеджеру соединения с Binance...")
	close(appShutdownSignalForBinanceMgr)

	// 2. Останавливаем HTTP сервер
	ctxHttp, cancelHttp := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelHttp()
	if err := httpSrv.Shutdown(ctxHttp); err != nil {
		log.Printf("Ошибка graceful shutdown HTTP сервера: %v", err)
	} else {
		log.Println("HTTP сервер остановлен.")
	}

	log.Println("Ожидание завершения работы менеджера Binance")
	wg.Wait()

	log.Println("Приложение завершено.")
	time.Sleep(1 * time.Second)
}

func setupApplication(cfg *config.AppConfig) (*sync.WaitGroup, *analysis.PriceProcessor, *alerter.WebAlerter, chan os.Signal) {
	wg := sync.WaitGroup{}
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	logAlerter := alerter.NewLogAlerter()
	webAlerter := alerter.NewWebAlerter()
	compositeAlerter := alerter.NewCompositeAlerter(logAlerter, webAlerter)
	priceProcessor := analysis.NewPriceProcessor(cfg, compositeAlerter)

	return &wg, priceProcessor, webAlerter, interrupt
}

func startHTTPServer(webAlerter *alerter.WebAlerter) *http.Server {
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/", fs)
	mux.Handle("/ws/alerts", webAlerter)

	srv := newServer(mux)

	log.Println("Запуск HTTP сервера на http://localhost:8080 ...")
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Критическая ошибка HTTP сервера: %v", err)
		}
	}()
	return srv
}

func newServer(mux *http.ServeMux) *http.Server {
	httpServer := &http.Server{
		Addr:           ":8080",
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}
	return httpServer
}

func connectAndSubscribeBinance(cfg *config.AppConfig) (*websocketclient.Client, *models.WebSocketRequest, error) {
	client, err := websocketclient.New(cfg.FuturesWebSocketURL)
	if err != nil {
		return nil, nil, fmt.Errorf("создание WebSocket клиента Binance: %w", err)
	}

	subReq, err := client.Subscribe(cfg.StreamName, 1)
	if err != nil {
		client.Close()
		return nil, nil, fmt.Errorf("подписка на поток Binance: %w", err)
	}
	log.Println("Успешная подписка на поток Binance для текущей сессии.")
	return client, subReq, nil
}

func processBinanceMessages(
	client *websocketclient.Client,
	subReq *models.WebSocketRequest, // Для проверки ID ответа на подписку
	priceProcessor *analysis.PriceProcessor,
	// Канал sessionDone сигнализирует этой функции, что нужно завершить текущую сессию
	// (например, при общем завершении приложения или перед попыткой переподключения)
	sessionDone chan struct{},
) error {
	log.Println("Горутина обработки сообщений Binance для текущей сессии запущена.")
	defer log.Println("Горутина обработки сообщений Binance для текущей сессии завершена.")

	// ReadMessages теперь принимает `done` канал, который мы будем использовать как sessionDone.
	// Когда client.Close() вызывается (например, менеджером соединений или при общем shutdown),
	// ReadMessages должна вернуть ошибку, что приведет к выходу из этой функции.
	messagesChan, errsChan := client.ReadMessages(sessionDone)

	for {
		select {
		case rawMessage, ok := <-messagesChan:
			if !ok {
				log.Println("Канал сообщений Binance закрыт (вероятно, сессия завершается).")
				return nil // Считаем, что это штатное завершение сессии по сигналу sessionDone
			}
			var response map[string]interface{}
			if err := json.Unmarshal(rawMessage, &response); err == nil {
				if idFloat, idOK := response["id"].(float64); idOK {
					if id := int(idFloat); id == subReq.ID {
						log.Printf("Ответ от сервера Binance на подписку: %s", rawMessage)
						continue
					}
				}
			}
			var tickers []models.MiniTicker
			if err := json.Unmarshal(rawMessage, &tickers); err == nil {
				priceProcessor.Process(tickers)
			} else {
				// log.Printf("Ошибка unmarshal []MiniTicker из Binance: %v. Сообщение: %s", err, rawMessage)
			}

		case err, ok := <-errsChan:
			if !ok {
				log.Println("Канал ошибок Binance закрыт (вероятно, сессия завершается).")
				return nil // Штатное завершение
			}
			if err != nil {
				log.Printf("Ошибка от Binance WebSocket клиента в текущей сессии: %v.", err)
				return err // Возвращаем ошибку, чтобы менеджер соединений знал о проблеме
			}
			// err == nil и ok == true - можно считать сигналом к завершению без ошибки
			return nil

		case <-sessionDone: // Если внешний сигнал на завершение именно этой сессии
			log.Println("Обработка сообщений Binance: получен сигнал sessionDone. Завершение текущей сессии.")
			return nil // Завершаемся без ошибки
		}
	}
}

func runBinanceConnectionManager(cfg *config.AppConfig, priceProcessor *analysis.PriceProcessor, appShutdownSignal chan struct{}) {
	log.Println("Менеджер соединения с Binance запущен.")
	defer log.Println("Менеджер соединения с Binance остановлен.")

	for {
		// Начальная проверка на выход, если сигнал пришел между сессиями
		select {
		case <-appShutdownSignal:
			log.Println("Менеджер Binance: получен сигнал на завершение перед новой попыткой.")
			return
		default:
		}

		log.Printf("Менеджер Binance: попытка подключения...")
		client, subReq, err := connectAndSubscribeBinance(cfg)
		if err != nil {
			log.Printf("Менеджер Binance: ошибка подключения: %v", err)
			time.Sleep(5 * time.Second) // Простая задержка перед повторной попыткой
			continue
		}

		// --- НОВАЯ ЛОГИКА УПРАВЛЕНИЯ СЕССИЕЙ ---

		currentSessionDone := make(chan struct{})   // Канал для остановки именно этой сессии
		processingResultChan := make(chan error, 1) // Канал для результата работы processBinanceMessages

		// Запускаем обработку сообщений в ДОЧЕРНЕЙ горутине
		go func() {
			log.Println("[Manager] Дочерняя горутина processBinanceMessages запущена.")
			err := processBinanceMessages(client, subReq, priceProcessor, currentSessionDone)
			processingResultChan <- err
			log.Println("[Manager] Дочерняя горутина processBinanceMessages завершена.")
		}()

		// Теперь менеджер СВОБОДЕН и может слушать сигналы в select
		log.Println("[Manager] Ожидание завершения сессии или сигнала shutdown...")

		select {
		case err := <-processingResultChan:
			// Сессия завершилась сама по себе (из-за ошибки или обрыва связи)
			if err != nil {
				log.Printf("Менеджер Binance: сессия завершилась с ошибкой: %v", err)
			} else {
				log.Println("Менеджер Binance: сессия завершилась штатно.")
			}
			// Цикл for просто перейдет к следующей итерации и попробует переподключиться

		case <-appShutdownSignal:
			// Пришел главный сигнал на завершение ВСЕГО приложения
			log.Println("Менеджер Binance: получен глобальный сигнал shutdown. Завершение текущей сессии...")

			// 1. Говорим `processBinanceMessages` остановиться
			close(currentSessionDone)

			// 2. Ждем, пока она действительно завершится и вернет результат
			err := <-processingResultChan
			log.Printf("Менеджер Binance: дочерняя горутина подтвердила остановку (результат: %v).", err)

			// 3. Закрываем соединение и выходим из менеджера
			client.Close()
			return // Выход из runBinanceConnectionManager
		}

		// Закрываем клиент после каждой сессии перед переподключением
		log.Println("Менеджер Binance: закрытие клиента после сессии...")
		client.Close()
		time.Sleep(5 * time.Second) // Пауза перед переподключением
	}
}
