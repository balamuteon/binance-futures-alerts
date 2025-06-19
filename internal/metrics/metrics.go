package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Счетчик текущего количества подключенных клиентов
	ConnectedClients = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "app_websocket_connected_clients",
		Help: "The current number of connected WebSocket clients.",
	})

	// Счетчик общего количества пришедших алертов
	AlertsSentTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "app_alerts_sent_total",
		Help: "The total number of alerts sent.",
	})

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "app_http_request_duration_seconds",
			Help: "Duration of HTTP requests.",
		},
		[]string{"path"}, // Метка, чтобы разделять метрики по разным URL
	)
)
