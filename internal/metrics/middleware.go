// internal/metrics/middleware.go
package metrics

import (
	"net/http"
	"time"
)

func PrometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r) // Вызываем следующий обработчик в цепочке
		duration := time.Since(start)

		// Записываем длительность в нашу гистограмму
		HTTPRequestDuration.WithLabelValues(r.URL.Path).Observe(duration.Seconds())
	})
}
