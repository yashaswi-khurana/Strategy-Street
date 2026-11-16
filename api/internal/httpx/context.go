package httpx

import (
	"context"
)

type contextKey string

const (
	requestIDContextKey contextKey = "request_id"
	HeaderRequestID                = "X-Request-Id"
)

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey, requestID)
}

func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if val, ok := ctx.Value(requestIDContextKey).(string); ok {
		return val
	}
	return ""
}
