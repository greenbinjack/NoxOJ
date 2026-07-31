package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"noxoj/internal/config"
	"noxoj/internal/health"
	"noxoj/internal/logging"
)

func requestLogger(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.Status()).
				Dur("duration", time.Since(start)).
				Msg("request handled")
		})
	}
}

func newRouter(logger zerolog.Logger) *chi.Mux {
	r := chi.NewRouter()
	r.Use(requestLogger(logger))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("NoxOJ API — Sprint 1 skeleton is alive"))
	})

	r.Get("/healthz", health.Liveness)
	r.Get("/readyz", health.Readiness())

	return r
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.Environment)

	r := newRouter(logger)

	addr := fmt.Sprintf(":%d", cfg.Port)
	logger.Info().Str("addr", addr).Str("environment", string(cfg.Environment)).Msg("NoxOJ API starting")

	if err := http.ListenAndServe(addr, r); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
