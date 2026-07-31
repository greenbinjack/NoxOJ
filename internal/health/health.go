// Package health provides liveness and readiness HTTP handlers.
package health

import "net/http"

// Liveness reports whether the process itself is running and able to
// handle a request at all. It never checks external dependencies —
// that's what Readiness is for. If Liveness starts failing, the
// process itself is broken and should be restarted.
func Liveness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// Checker reports an error if some dependency the app relies on isn't
// currently usable (e.g. the database is unreachable).
type Checker func() error

// Readiness reports whether this instance should currently receive
// traffic — true once every registered Checker passes. There are no
// checkers yet, since nothing in the app depends on an external
// service today; it behaves identically to Liveness for now. Sprint 7+
// adds a database Checker here once the app actually talks to Postgres.
func Readiness(checks ...Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for _, check := range checks {
			if err := check(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("not ready: " + err.Error()))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}
}
