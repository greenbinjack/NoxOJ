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
	"noxoj/internal/database"
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

// newRouter takes readiness checks as plain health.Checker functions,
// not a *sqlx.DB directly — so route tests that have nothing to do
// with the database (TestRootRoute, TestHealthzRoute) never need a
// live Postgres connection just to construct a router.
func newRouter(logger zerolog.Logger, readinessChecks ...health.Checker) *chi.Mux {
	r := chi.NewRouter()
	r.Use(requestLogger(logger))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("NoxOJ API — Sprint 1 skeleton is alive"))
	})

	r.Get("/healthz", health.Liveness)
	r.Get("/readyz", health.Readiness(readinessChecks...))

	return r
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.Environment)

	db, err := database.Connect(database.Config{
		Host:     cfg.Postgres.Host,
		Port:     cfg.Postgres.Port,
		User:     cfg.Postgres.User,
		Password: cfg.Postgres.Password,
		Name:     cfg.Postgres.Name,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	r := newRouter(logger, database.Checker(db))

	addr := fmt.Sprintf(":%d", cfg.Port)
	logger.Info().Str("addr", addr).Str("environment", string(cfg.Environment)).Msg("NoxOJ API starting")

	if err := http.ListenAndServe(addr, r); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
