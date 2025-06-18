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
	"time"
	"sync"
)


func setupApplication(cfg *config.AppConfig) (*analysis.PriceProcessor, *alerter.WebAlerter, chan os.Signal) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	logAlerter := alerter.NewLogAlerter()
	webAlerter := alerter.NewWebAlerter()
	compositeAlerter := alerter.NewCompositeAlerter(logAlerter, webAlerter)
	priceProcessor := analysis.NewPriceProcessor(cfg, compositeAlerter)

	return priceProcessor, webAlerter, interrupt
}

func startHTTPServer(webAlerter *alerter.WebAlerter) *http.Server {
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/", fs)
	mux.Handle("/ws/alerts", webAlerter)

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Println("Запуск HTTP сервера на http://localhost:8080 ...")
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Критическая ошибка HTTP сервера: %v", err)
		}
	}()
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

	var currentClient *websocketclient.Client
	var currentSessionDone chan struct{}

	// Перед выходом из менеджера убедимся, что текущее соединение закрыто
	defer func() {
		if currentClient != nil {
			log.Println("Менеджер Binance: закрытие клиента при выходе из менеджера...")
			if currentSessionDone != nil {
				close(currentSessionDone) // Сигнализируем обработчику сообщений остановиться
			}
			currentClient.Close()
		}
	}()

	retryDelay := 5 * time.Second // Начальная задержка перед повторной попыткой

	for {
		// Проверяем, не нужно ли завершить работу менеджера
		select {
		case <-appShutdownSignal:
			log.Println("Менеджер Binance: получен сигнал на полное завершение.")
			return
		default:
			// Продолжаем работу
		}

		log.Printf("Менеджер Binance: попытка подключения (следующая через ~%s, если была ошибка)...", retryDelay.String())
		client, subReq, err := connectAndSubscribeBinance(cfg)
		if err != nil {
			log.Printf("Менеджер Binance: не удалось подключиться/подписаться: %v.", err)
			// Ожидаем перед следующей попыткой, но также слушаем сигнал завершения
			select {
			case <-time.After(retryDelay):
				if retryDelay < 60*time.Second { // Увеличиваем задержку, но не более минуты
					retryDelay += 5 * time.Second
				}
				continue // Переходим к следующей итерации цикла for для новой попытки
			case <-appShutdownSignal:
				log.Println("Менеджер Binance: получен сигнал на полное завершение во время ожидания повторной попытки.")
				return
			}
		}

		// Успешное подключение
		currentClient = client
		currentSessionDone = make(chan struct{}) // Канал для управления текущей сессией
		log.Println("Менеджер Binance: успешно подключен к Binance.")
		retryDelay = 5 * time.Second // Сбрасываем задержку при успешном подключении

		// Запускаем обработку сообщений для этой сессии. Эта функция будет блокирующей.
		// Она вернется, когда сессия завершится (из-за ошибки или сигнала appShutdownSignal).
		processingErr := processBinanceMessages(currentClient, subReq, priceProcessor, currentSessionDone)

		// Сессия завершена, закрываем текущий клиент перед следующей попыткой (если она будет)
		log.Println("Менеджер Binance: текущая сессия с Binance завершена. Закрытие клиента...")
		currentClient.Close() // ReadMessages внутри processBinanceMessages должен завершиться из-за этого
		currentClient = nil
		// currentSessionDone уже должен был быть обработан или закрыт, если processingErr == nil

		if processingErr != nil {
			log.Printf("Менеджер Binance: сессия завершилась с ошибкой: %v.", processingErr)
			// Цикл for автоматически перейдет к следующей попытке подключения после задержки
		} else {
			// Если processingErr == nil, значит processBinanceMessages завершился по сигналу sessionDone.
			// Это обычно происходит при штатном завершении приложения.
			// Проверяем еще раз сигнал appShutdownSignal, чтобы не войти в бесконечный цикл переподключений при остановке.
			select {
			case <-appShutdownSignal:
				log.Println("Менеджер Binance: подтверждено завершение после чистой остановки сессии.")
				return
			default:
				// Если не было ошибки и не было сигнала на выход, возможно, что-то пошло не так
				// или это был какой-то другой чистый выход. Продолжим с переподключением.
				log.Println("Менеджер Binance: сессия завершилась без ошибки, но без сигнала на выход. Попытка переподключения...")
			}
		}
		// Ожидаем перед следующей попыткой подключения
		select {
		case <-time.After(retryDelay):
			if retryDelay < 60*time.Second {
				retryDelay += 5 * time.Second
			}
		case <-appShutdownSignal:
			log.Println("Менеджер Binance: получен сигнал на полное завершение во время ожидания перед переподключением.")
			return
		}
	}
}

// Обновленная функция main
func main() {
	cfg := config.Load()
	wg := sync.WaitGroup{}
	priceProcessor, webAlerter, interruptChan := setupApplication(cfg)

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
}
