package alerter

import (
	"log"
	"math"
)

type LogAlerter struct{}

func NewLogAlerter() *LogAlerter {
	return &LogAlerter{}
}

func (la *LogAlerter) Alert(symbol string, percentageChange float64, currentPrice float64, oldestPrice float64) {
	var directionText string
	if percentageChange > 0 {
		directionText = "выросла на"
	} else {
		directionText = "упала на"
	}

	log.Printf(
		"!!! ALERT !!! Символ: %s, Цена %s %.2f%%. Текущая: %.4f, Старая в окне: %.4f",
		symbol,
		directionText,
		math.Abs(percentageChange), // Используем math.Abs для отображения абсолютного значения процента
		currentPrice,
		oldestPrice,
	)
}