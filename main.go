package main

import (
	"Aegis/internal/config"
	"Aegis/internal/proxy"
	"log"
	"net/http"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Create proxy
	p, err := proxy.New(cfg.Upstream, cfg.Timeout, cfg.TTL, cfg.Cache.KeyHeaders)
	if err != nil {
		log.Fatalf("init proxy: %v", err)
	}

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/stats", p.StatsHandler)
	mux.Handle("/", p)

	// Start server
	log.Printf("listening on %s, upstream %s, ttl=%s, timeout=%s",
		cfg.Listen, cfg.Upstream, cfg.TTL.String(), cfg.Timeout.String())
	if len(cfg.Cache.KeyHeaders) > 0 {
		log.Printf("cache key includes headers: %v", cfg.Cache.KeyHeaders)
	}
	if err := http.ListenAndServe(cfg.Listen, mux); err != nil {
		log.Fatal(err)
	}
}
