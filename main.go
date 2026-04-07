package main

import (
	"embed"
	"log"
	"net/http"
	"time"
)

//go:embed static/*
var staticFS embed.FS

//go:embed templates/*
var templateFS embed.FS

func main() {
	cfg := parseFlags()

	store, err := NewStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer store.Close()

	// Configure TLS for app detection/completions
	tlsSkipVerify = cfg.TLSSkipVerify

	// Start background completion refresher for app integrations
	StartCompletionRefresher(store, 1*time.Hour)

	srv := newServer(store, cfg)
	handler := srv.handler()

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("Go Links server listening on %s", cfg.Addr)
	log.Fatal(server.ListenAndServe())
}
