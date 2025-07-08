package alerter

import "binance/internal/generator/analysis"

type CompositeAlerter struct {
	alerters []analysis.Alerter
}

func NewCompositeAlerter(alerters ...analysis.Alerter) *CompositeAlerter {
	return &CompositeAlerter{alerters: alerters}
}

func (ca *CompositeAlerter) Add(alerter analysis.Alerter) {
	ca.alerters = append(ca.alerters, alerter)
}

func (ca *CompositeAlerter) Alert(symbol string, percentageChange float64, currentPrice float64, oldestPrice float64) {
	for _, a := range ca.alerters {
		a.Alert(symbol, percentageChange, currentPrice, oldestPrice)
	}
}
