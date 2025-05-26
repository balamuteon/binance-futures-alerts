package analysis

import (
	"container/list"
	"log"
	"binance/internal/config"
	"binance/internal/models"
	"math"
	"strconv"
	"sync"
	"time"
)

// Alerter интерфейс для отправки уведомлений
type Alerter interface {
	Alert(symbol string, percentageChange float64, currentPrice float64, oldestPrice float64)
}

type PriceProcessor struct {
	config             *config.AppConfig
	symbolPriceHistory map[string]*list.List
	lastAlertTimestamp map[string]int64
	historyMutex       *sync.Mutex
	alerter            Alerter
}

func NewPriceProcessor(cfg *config.AppConfig, alerter Alerter) *PriceProcessor {
	return &PriceProcessor{
		config:             cfg,
		symbolPriceHistory: make(map[string]*list.List),
		lastAlertTimestamp: make(map[string]int64),
		historyMutex:       &sync.Mutex{},
		alerter:            alerter,
	}
}

// Process принимает срез MiniTicker и выполняет анализ
func (pp *PriceProcessor) Process(tickers []models.MiniTicker) {
	pp.historyMutex.Lock()
	defer pp.historyMutex.Unlock()

	currentTimeMillis := time.Now().UnixMilli() // Для cooldown алертов

	for _, ticker := range tickers {
		currentPriceFloat, err := strconv.ParseFloat(ticker.ClosePrice, 64)
		if err != nil {
			log.Printf("[Processor] Ошибка парсинга цены для %s ('%s'): %v", ticker.Symbol, ticker.ClosePrice, err)
			continue
		}
		if math.IsNaN(currentPriceFloat) || currentPriceFloat <= 0 {
			log.Printf("[Processor] Некорректная цена для %s: %f", ticker.Symbol, currentPriceFloat)
			continue
		}

		currentTickerTimestamp := ticker.EventTime

		priceList, exists := pp.symbolPriceHistory[ticker.Symbol]
		if !exists {
			priceList = list.New()
			pp.symbolPriceHistory[ticker.Symbol] = priceList
		}

		priceList.PushFront(models.PricePoint{Timestamp: currentTickerTimestamp, Price: currentPriceFloat})

		cutoffTimestamp := currentTickerTimestamp - pp.config.PriceHistoryDuration.Milliseconds()
		element := priceList.Back()
		for element != nil {
			prevElement := element.Prev()
			point := element.Value.(models.PricePoint)
			if point.Timestamp < cutoffTimestamp {
				priceList.Remove(element)
			} else {
				break
			}
			element = prevElement
		}

		if priceList.Len() > 1 {
			oldestPointInWindow := priceList.Back().Value.(models.PricePoint)
			if oldestPointInWindow.Price > 0 {
				percentageChange := ((currentPriceFloat - oldestPointInWindow.Price) / oldestPointInWindow.Price) * 100.0

				alertConditionMet := false
				
				// Проверяем РОСТ
				if percentageChange > pp.config.PercentageThreshold {
					alertConditionMet = true
				} else if percentageChange < -pp.config.PercentageThreshold { // Проверяем ПАДЕНИЕ
					// percentageChange уже будет отрицательным, например -2.5%
					// -pp.config.PercentageThreshold будет, например, -1.0% (если PercentageThreshold = 1.0)
					// Условие -2.5 < -1.0 будет истинным.
					alertConditionMet = true
				}

				if alertConditionMet {
					if lastAlertTs, alerted := pp.lastAlertTimestamp[ticker.Symbol]; !alerted || (currentTimeMillis-lastAlertTs > pp.config.AlertCooldown.Milliseconds()) {
						// Передаем percentageChange как есть (он будет положительным для роста, отрицательным для падения)
						pp.alerter.Alert(ticker.Symbol, percentageChange, currentPriceFloat, oldestPointInWindow.Price)
						pp.lastAlertTimestamp[ticker.Symbol] = currentTimeMillis
					}
				}
			}
		}
	}
}
