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
		FuturesWebSocketURL:  "wss://fstream.binance.com/ws",
		StreamName:           "!miniTicker@arr",
		PriceHistoryDuration: 1 * time.Minute,
		PercentageThreshold:  0.3, // adjust this as needed
		AlertCooldown:        30 * time.Second,
	}
}
