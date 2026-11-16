package httpx

import (
	"context"
	"io"
	"log/slog"
)

type RequestIDHandler struct {
	inner slog.Handler
}

func NewJSONLogger(w io.Writer, level slog.Level) *slog.Logger {
	handler := &RequestIDHandler{
		inner: slog.NewJSONHandler(w, &slog.HandlerOptions{
			Level: level,
		}),
	}
	return slog.New(handler)
}

func (h *RequestIDHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *RequestIDHandler) Handle(ctx context.Context, r slog.Record) error {
	if reqID := GetRequestID(ctx); reqID != "" {
		hasReqID := false
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "request_id" {
				hasReqID = true
				return false
			}
			return true
		})
		if !hasReqID {
			r.AddAttrs(slog.String("request_id", reqID))
		}
	}
	return h.inner.Handle(ctx, r)
}

func (h *RequestIDHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &RequestIDHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *RequestIDHandler) WithGroup(name string) slog.Handler {
	return &RequestIDHandler{inner: h.inner.WithGroup(name)}
}
