package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/strategy-street/api/internal/httpx"
)

type mockReadinessChecker struct {
	dbErr    error
	redisErr error
}

func (m *mockReadinessChecker) PingDatabase(ctx context.Context) error {
	return m.dbErr
}

func (m *mockReadinessChecker) PingRedis(ctx context.Context) error {
	return m.redisErr
}

func testLogger(buf io.Writer) *slog.Logger {
	return httpx.NewJSONLogger(buf, slog.LevelDebug)
}

func TestHealthz(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)
	router := httpx.NewRouter(logger, &mockReadinessChecker{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res["status"] != "ok" {
		t.Fatalf("expected status 'ok', got %q", res["status"])
	}

	reqIDHeader := rec.Header().Get("X-Request-Id")
	if reqIDHeader == "" {
		t.Fatalf("expected X-Request-Id header to be set")
	}
}

func TestReadyz(t *testing.T) {
	tests := []struct {
		name           string
		checker        httpx.ReadinessChecker
		expectedStatus int
		expectedState  string
	}{
		{
			name: "both healthy",
			checker: &mockReadinessChecker{
				dbErr:    nil,
				redisErr: nil,
			},
			expectedStatus: http.StatusOK,
			expectedState:  "ready",
		},
		{
			name: "database down",
			checker: &mockReadinessChecker{
				dbErr:    errors.New("db connection refused"),
				redisErr: nil,
			},
			expectedStatus: http.StatusServiceUnavailable,
			expectedState:  "unavailable",
		},
		{
			name: "redis down",
			checker: &mockReadinessChecker{
				dbErr:    nil,
				redisErr: errors.New("redis connection refused"),
			},
			expectedStatus: http.StatusServiceUnavailable,
			expectedState:  "unavailable",
		},
		{
			name: "both down",
			checker: &mockReadinessChecker{
				dbErr:    errors.New("db down"),
				redisErr: errors.New("redis down"),
			},
			expectedStatus: http.StatusServiceUnavailable,
			expectedState:  "unavailable",
		},
		{
			name:           "nil checker",
			checker:        nil,
			expectedStatus: http.StatusServiceUnavailable,
			expectedState:  "unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := testLogger(&buf)
			router := httpx.NewRouter(logger, tt.checker)

			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			var res httpx.ReadyResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
				t.Fatalf("failed to decode json: %v", err)
			}

			if res.Status != tt.expectedState {
				t.Fatalf("expected status %q, got %q", tt.expectedState, res.Status)
			}
		})
	}
}

func TestRequestIDEchoAndPropagation(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)
	router := httpx.NewRouter(logger, &mockReadinessChecker{})

	customReqID := "custom-trace-uuid-1234"
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-Id", customReqID)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-Id") != customReqID {
		t.Fatalf("expected X-Request-Id %q, got %q", customReqID, rec.Header().Get("X-Request-Id"))
	}

	// Verify log line is JSON and contains request_id
	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("expected valid JSON log output, got error: %v, raw: %s", err, buf.String())
	}

	if logEntry["request_id"] != customReqID {
		t.Fatalf("expected log entry request_id %q, got %v", customReqID, logEntry["request_id"])
	}
}

func TestErrorEnvelopeOnNotFound(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)
	router := httpx.NewRouter(logger, &mockReadinessChecker{})

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	req.Header.Set("X-Request-Id", "err-test-404")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	var envelope httpx.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse error envelope: %v", err)
	}

	if envelope.RequestID != "err-test-404" {
		t.Fatalf("expected request_id 'err-test-404', got %q", envelope.RequestID)
	}

	if envelope.Error.Code != "not_found" {
		t.Fatalf("expected error code 'not_found', got %q", envelope.Error.Code)
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to parse raw json: %v", err)
	}
	errObj, ok := raw["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object in json response")
	}
	if _, ok := errObj["details"]; !ok {
		t.Fatalf("expected 'details' field in error object, got %v", errObj)
	}
}
