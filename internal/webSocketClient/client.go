// internal/websocketclient/client.go
package websocketclient

import (
	"log"
	"net/url"
	"binance/internal/models" // Замени project_name на имя твоего модуля из go.mod

	"github.com/gorilla/websocket"
)

type Client struct {
	conn *websocket.Conn
	url  string
}

func New(wsURL string) (*Client, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		log.Printf("Ошибка парсинга URL '%s': %v", wsURL, err)
		return nil, err
	}
	log.Printf("Подключаемся к %s...", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Printf("Ошибка подключения Dial: %v", err)
		return nil, err
	}
	log.Println("Успешно подключились к WebSocket.")
	return &Client{conn: c, url: wsURL}, nil
}

func (c *Client) Subscribe(streamName string, requestID int) (*models.WebSocketRequest, error) {
	payload := models.WebSocketRequest{
		Method: "SUBSCRIBE",
		Params: []string{streamName},
		ID:     requestID,
	}
	log.Printf("Отправка запроса на подписку: %+v", payload)
	err := c.conn.WriteJSON(payload)
	if err != nil {
		log.Printf("Ошибка отправки JSON для подписки: %v", err)
		return nil, err
	}
	return &payload, nil // Возвращаем payload, чтобы ID был доступен для проверки ответа
}

// ReadMessages читает сообщения из WebSocket и отправляет их в канал.
// Канал байтовых срезов, так как парсинг - это уже другая ответственность.
func (c *Client) ReadMessages(done chan struct{}) (<-chan []byte, <-chan error) {
	messages := make(chan []byte)
	errs := make(chan error, 1) // Буферизированный канал для ошибок

	go func() {
		defer close(messages)
		defer close(errs)
		// defer c.conn.Close() // Закрытие соединения лучше обрабатывать в main или при вызове метода Close() у клиента

		for {
			select {
			case <-done: // Если пришел сигнал о завершении
				log.Println("WebSocket клиент: получен сигнал 'done', завершение чтения.")
				return
			default: // Продолжаем читать сообщения
				messageType, message, err := c.conn.ReadMessage()
				if err != nil {
					// Проверка, что это не ошибка из-за закрытия по сигналу "done"
					// (в этом случае done уже должен был быть обработан)
					log.Printf("WebSocket клиент: ошибка чтения сообщения: %v", err)
					errs <- err
					return // Завершаем горутину при ошибке чтения
				}

				// Отправляем только текстовые сообщения дальше, обрабатываем Ping
				if messageType == websocket.TextMessage {
					messages <- message
				} else if messageType == websocket.PingMessage {
					log.Println("WebSocket клиент: получен Ping, отправляем Pong.")
					if err = c.conn.WriteMessage(websocket.PongMessage, nil); err != nil {
						log.Printf("WebSocket клиент: ошибка отправки Pong: %v", err)
						errs <- err
						return
					}
				}
			}
		}
	}()
	return messages, errs
}

func (c *Client) WriteMessage(messageType int, data []byte) error {
	return c.conn.WriteMessage(messageType, data)
}

func (c *Client) Close() error {
	log.Println("WebSocket клиент: Закрытие соединения.")
	// Отправляем серверу сообщение о закрытии
	err := c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		log.Printf("WebSocket клиент: ошибка отправки CloseMessage: %v", err)
		// Все равно пытаемся закрыть соединение локально
		return c.conn.Close()
	}
	// Также закрываем локальное соединение
	return c.conn.Close()
}