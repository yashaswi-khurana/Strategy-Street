package httpx

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter(logger *slog.Logger, checker ReadinessChecker) chi.Router {
	r := chi.NewRouter()

	r.Use(RequestIDMiddleware)
	r.Use(LoggerMiddleware(logger))
	r.Use(RecovererMiddleware(logger))

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		WriteError(w, req, http.StatusNotFound, "not_found", "endpoint not found", nil)
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		WriteError(w, req, http.StatusMethodNotAllowed, "method_not_allowed", "http method not allowed", nil)
	})

	r.Get("/healthz", HealthzHandler)
	r.Get("/readyz", ReadyzHandler(checker))

	return r
}
