package handlers

import (
	"binance/internal/alerter"
	"net/http"
	"net/http/pprof"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewHandler(webAlerter *alerter.WebAlerter) http.Handler {
	mux := http.NewServeMux()
	
	fs := http.FileServer(http.Dir("./static"))

	mux.Handle("/", fs)
	mux.Handle("/ws/alerts", webAlerter)
	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return mux
}
