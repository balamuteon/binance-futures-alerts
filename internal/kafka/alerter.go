package kafka

import (
	"binance/internal/models"
	"context"
	"encoding/json"
	"log"
	"time"

	kafkaGO "github.com/segmentio/kafka-go"
)

type KafkaAlerter struct {
	Writer *kafkaGO.Writer
}

// Alert реализует интерфейс analysis.Alerter
func (ka *KafkaAlerter) Alert(symbol string, percentageChange float64, currentPrice float64, oldestPrice float64) {
	alertMsg := models.AlertMessage{
		Symbol:           symbol,
		PercentageChange: percentageChange,
		CurrentPrice:     currentPrice,
		OldestPrice:      oldestPrice,
		Timestamp:        time.Now(),
	}

	// Кодируем алерт в JSON
	jsonMsg, err := json.Marshal(alertMsg)
	if err != nil {
		log.Printf("[Generator] Ошибка кодирования алерта в JSON: %v", err)
		return
	}

	// Отправляем JSON в топик system_alerts
	err = ka.Writer.WriteMessages(context.Background(), kafkaGO.Message{
		Value: jsonMsg,
	})
	if err != nil {
		log.Printf("[Generator] Ошибка записи алерта в Kafka: %v", err)
	} else {
		// log.Printf("[Generator] >>> Алерт для %s отправлен в Kafka!", symbol)
	}
}
