package main

import (
	"embed"
	"log"
	"net/http"
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

	srv := newServer(store, cfg)
	handler := srv.handler()

	log.Printf("Go Links server listening on %s", cfg.Addr)
	log.Fatal(http.ListenAndServe(cfg.Addr, handler))
}
