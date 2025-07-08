package config

import "time"

type AppConfig struct {
	FuturesWebSocketURL  string
	StreamName           string
	PriceHistoryDuration time.Duration
	PercentageThreshold  float64
	AlertCooldown        time.Duration
}

func Load() *AppConfig {
	return &AppConfig{
		FuturesWebSocketURL:  "wss://fstream.binance.com/ws", // URL для WebSocket Binance Futures
		StreamName:           "!miniTicker@arr",              // Имя стрима для получения мини-тикеров
		PriceHistoryDuration: 1 * time.Minute,                // Длительность хранения истории цен, за которую будет производиться анализ
		PercentageThreshold:  2,                              // Порог изменения цены в процентах(%) для срабатывания алерта
		AlertCooldown:        30 * time.Second,               // Время ожидания перед следующей проверкой алерта для одного и того же тикера
	}
}
