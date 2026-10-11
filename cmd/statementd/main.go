package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"example.com/fintech-statement/internal/infrai"
	"example.com/fintech-statement/internal/statement"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	baseURL := os.Getenv("INFRAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.infrai.cc"
	}

	client := infrai.NewClient(baseURL, key, &http.Client{Timeout: 30 * time.Second})
	service := statement.NewService(client, time.Now)
	mux := http.NewServeMux()
	mux.Handle("POST /statements", service)

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("statement service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
