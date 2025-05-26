package models

import (
	"time"
)

type AlertMessage struct {
	Symbol           string    `json:"symbol"`
	PercentageChange float64   `json:"percentageChange"`
	CurrentPrice     float64   `json:"currentPrice"`
	OldestPrice      float64   `json:"oldestPrice"`
	Timestamp        time.Time `json:"timestamp"`
}

// Структура для запроса на подписку/отписку к WebSocket серверу
type WebSocketRequest struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
	ID     int      `json:"id"`
}

// MiniTicker представляет данные одного тикера из потока !miniTicker@arr
type MiniTicker struct {
	Symbol      string `json:"s"` // Symbol
	ClosePrice  string `json:"c"` // Close price
	OpenPrice   string `json:"o"` // Open price
	HighPrice   string `json:"h"` // High price
	LowPrice    string `json:"l"` // Low price
	Volume      string `json:"v"` // Total traded base asset volume
	QuoteVolume string `json:"q"` // Total traded quote asset volume
	EventType   string `json:"e"` // Event type
	EventTime   int64  `json:"E"` // Event time
}

// PricePoint хранит цену и временную метку для анализа истории
type PricePoint struct {
	Timestamp int64   // Unix миллисекунды
	Price     float64 // Цена
}
