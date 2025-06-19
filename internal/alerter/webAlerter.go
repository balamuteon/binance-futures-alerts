package alerter

import (
	"binance/internal/models"
	"binance/internal/metrics"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WebAlerter struct {
	clients   map[*websocket.Conn]bool // Пул активных веб-клиентов
	mu        sync.Mutex               // Мьютекс для защиты clients
	upgrader  websocket.Upgrader       // Для "апгрейда" HTTP до WebSocket
	broadcast chan models.AlertMessage // Канал для рассылки алертов
	shutdown  chan struct{}
	wg        sync.WaitGroup
}

func NewWebAlerter() *WebAlerter {
	wa := &WebAlerter{
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			// Для простоты разрешаем все источники. В продакшене нужно настроить!
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		broadcast: make(chan models.AlertMessage, 256), // Буферизированный канал
		shutdown:  make(chan struct{}),
	}
	wa.wg.Add(1)
	go wa.runBroadcaster() // Запускаем горутину, которая будет рассылать сообщения клиентам
	return wa
}

// ServeHTTP делает WebAlerter обработчиком HTTP запросов для WebSocket
func (wa *WebAlerter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := wa.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebAlerter] Ошибка апгрейда до WebSocket: %v", err)
		return
	}
	// Регистрация нового клиента
	wa.mu.Lock()
	wa.clients[conn] = true
	log.Printf("[WebAlerter] Новый веб-клиент подключен. Всего клиентов: %d", len(wa.clients))
	wa.mu.Unlock()
	metrics.ConnectedClients.Inc() // metrics increment

	go wa.handleClientReads(conn)
}

func (wa *WebAlerter) handleClientReads(conn *websocket.Conn) {
	defer func() {
		wa.mu.Lock()
		delete(wa.clients, conn)
		log.Printf("[WebAlerter] Веб-клиент отключился. Всего клиентов: %d", len(wa.clients))
		wa.mu.Unlock()
		conn.Close()
		metrics.ConnectedClients.Dec() // metrics decrement
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (wa *WebAlerter) Alert(symbol string, percentageChange float64, currentPrice float64, oldestPrice float64) {
	msg := models.AlertMessage{
		Symbol:           symbol,
		PercentageChange: percentageChange,
		CurrentPrice:     currentPrice,
		OldestPrice:      oldestPrice,
		Timestamp:        time.Now(),
	}
	select {
	case wa.broadcast <- msg:
	default:
		log.Printf("[WebAlerter] Канал broadcast переполнен. Сообщение для %s отброшено.", symbol)
	}
}

func (wa *WebAlerter) runBroadcaster() {
	defer wa.wg.Done() // <-- Сигнализируем о завершении, когда выйдем из функции
	
	for {
		select {
		case alertMsg, ok := <-wa.broadcast:
			if !ok {
				return
			}
			wa.mu.Lock()
			currentClients := make([]*websocket.Conn, 0, len(wa.clients))
			for client := range wa.clients {
				currentClients = append(currentClients, client)
				metrics.AlertsSentTotal.Inc() // metrics increment
			}
			wa.mu.Unlock()

			for _, client := range currentClients {
				err := client.WriteJSON(alertMsg)
				if err != nil {
					log.Printf("[WebAlerter] Ошибка записи JSON: %v. Удаление клиента.", err)
					client.Close()
					wa.mu.Lock()
					delete(wa.clients, client)
					wa.mu.Unlock()
				}
			}
		
		case <-wa.shutdown:
			log.Println("[WebAlerter] Broadcaster получил сигнал shutdown.")
			return
		}
	}
}

func (wa *WebAlerter) Shutdown() {
	log.Println("[WebAlerter] Запуск процедуры остановки...")
	close(wa.shutdown)
	// Ждем, пока горутина runBroadcaster действительно завершится
	wa.wg.Wait()
	log.Println("[WebAlerter] Остановлен.")
}