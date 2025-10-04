package main

import (
	"Aegis/internal/config"
	"Aegis/internal/logger"
	"Aegis/internal/proxy"
	"log"
	"net/http"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Create logger
	appLogger := logger.New(cfg.Logging.Enabled, cfg.Logging.AccessLog, cfg.Logging.Level)

	// Create proxy
	p, err := proxy.New(cfg.Upstream, cfg.Timeout, cfg.TTL, cfg.Cache.KeyHeaders, appLogger)
	if err != nil {
		log.Fatalf("init proxy: %v", err)
	}

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/stats", p.StatsHandler)
	mux.Handle("/", p)

	// Wrap with access log middleware
	var handler http.Handler = mux
	if cfg.Logging.Enabled && cfg.Logging.AccessLog {
		handler = appLogger.AccessLogMiddleware(mux)
	}

	// Start server
	log.Printf("listening on %s, upstream %s, ttl=%s, timeout=%s",
		cfg.Listen, cfg.Upstream, cfg.TTL.String(), cfg.Timeout.String())
	if len(cfg.Cache.KeyHeaders) > 0 {
		log.Printf("cache key includes headers: %v", cfg.Cache.KeyHeaders)
	}
	if cfg.Logging.Enabled {
		log.Printf("logging enabled: level=%s access_log=%v", cfg.Logging.Level, cfg.Logging.AccessLog)
	}
	if err := http.ListenAndServe(cfg.Listen, handler); err != nil {
		log.Fatal(err)
	}
}
