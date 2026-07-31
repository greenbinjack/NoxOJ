package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"noxoj/internal/config"
)

func newRouter() *chi.Mux {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("NoxOJ API — Sprint 1 skeleton is alive"))
	})

	return r
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	r := newRouter()

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("NoxOJ API listening on %s (environment=%s)", addr, cfg.Environment)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
