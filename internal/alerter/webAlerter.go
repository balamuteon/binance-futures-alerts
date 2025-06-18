package alerter

import (
	"binance/internal/models"
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
	}
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
	for alertMsg := range wa.broadcast {
		wa.mu.Lock()
		for client := range wa.clients {
			err := client.WriteJSON(alertMsg)
			if err != nil {
				log.Printf("[WebAlerter] Ошибка записи JSON веб-клиенту: %v. Удаление клиента.", err)
				client.Close()
				delete(wa.clients, client)
			}
		}
		wa.mu.Unlock()
	}
}
