package httpx

import (
	"context"
	"net/http"
	"time"
)

type ReadinessChecker interface {
	PingDatabase(ctx context.Context) error
	PingRedis(ctx context.Context) error
}

type HealthResponse struct {
	Status string `json:"status"`
}

type ReadyResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Redis    string `json:"redis"`
}

func HealthzHandler(w http.ResponseWriter, r *http.Request) {
	_ = WriteJSON(w, http.StatusOK, HealthResponse{
		Status: "ok",
	})
}

func ReadyzHandler(checker ReadinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		dbStatus := "ok"
		redisStatus := "ok"
		isReady := true

		if checker != nil {
			if err := checker.PingDatabase(ctx); err != nil {
				dbStatus = err.Error()
				isReady = false
			}

			if err := checker.PingRedis(ctx); err != nil {
				redisStatus = err.Error()
				isReady = false
			}
		} else {
			dbStatus = "not configured"
			redisStatus = "not configured"
			isReady = false
		}

		if !isReady {
			_ = WriteJSON(w, http.StatusServiceUnavailable, ReadyResponse{
				Status:   "unavailable",
				Database: dbStatus,
				Redis:    redisStatus,
			})
			return
		}

		_ = WriteJSON(w, http.StatusOK, ReadyResponse{
			Status:   "ready",
			Database: dbStatus,
			Redis:    redisStatus,
		})
	}
}
