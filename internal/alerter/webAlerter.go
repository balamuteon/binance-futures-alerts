// internal/alerter/webAlerter.go
package alerter

import (
	"binance/internal/metrics"
	"binance/internal/models"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket" // <--- ИСПРАВЛЕННЫЙ ИМПОРТ
)

type WebAlerter struct {
	clients  map[*websocket.Conn]bool
	mu       sync.RWMutex
	upgrader websocket.Upgrader

	broadcast chan models.AlertMessage
	shutdown  chan struct{}
	wg        sync.WaitGroup
}

func NewWebAlerter() *WebAlerter {
	wa := &WebAlerter{
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
		broadcast: make(chan models.AlertMessage, 256),
		shutdown:  make(chan struct{}),
	}
	wa.wg.Add(1)
	go wa.runBroadcaster()
	return wa
}

func (wa *WebAlerter) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	log.Println("[WebAlerter] >> ПОЛУЧЕН ЗАПРОС НА ПОДКЛЮЧЕНИЕ")

	conn, err := wa.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebAlerter] Ошибка апгрейда до WebSocket: %v", err)
		return
	}

	log.Println("[WebAlerter] >> Рукопожатие (upgrade) успешно завершено. Соединение создано.")

	wa.mu.Lock()
	wa.clients[conn] = true
	log.Printf("[WebAlerter] Новый веб-клиент подключен. Всего клиентов: %d", len(wa.clients))
	wa.mu.Unlock()
	metrics.ConnectedClients.Inc()

	log.Println("[WebAlerter] >> Клиент сохранен в sync.Map, запуск горутины handleClientReads...")

	go wa.handleClientReads(conn)

	log.Println("[WebAlerter] >> Обработчик ServeHTTP завершает работу для этого запроса.")

}

func (wa *WebAlerter) handleClientReads(conn *websocket.Conn) {
	defer func() {
		wa.mu.Lock()
		delete(wa.clients, conn)
		log.Printf("[WebAlerter] Веб-клиент отключился. Всего клиентов: %d", len(wa.clients))
		wa.mu.Unlock()
		conn.Close()
		metrics.ConnectedClients.Dec()
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
	defer wa.wg.Done()

	for {
		select {
		case alertMsg, ok := <-wa.broadcast:
			if !ok {
				return
			}
			wa.mu.RLock()
			currentClients := make([]*websocket.Conn, 0, len(wa.clients))
			for client := range wa.clients {
				currentClients = append(currentClients, client)
				metrics.AlertsSentTotal.Inc()
			}
			wa.mu.RUnlock()

			for _, client := range currentClients {
				err := client.WriteJSON(alertMsg)
				if err != nil {
					log.Printf("[WebAlerter] Ошибка записи JSON: %v. Удаление клиента.", err)
					wa.mu.Lock()
					delete(wa.clients, client)
					wa.mu.Unlock()
					client.Close()
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
	wa.wg.Wait()
	log.Println("[WebAlerter] Остановлен.")
}
