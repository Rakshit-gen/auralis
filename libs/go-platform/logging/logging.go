// Package logging configures structured JSON logging with the field set the
// platform standardises on: timestamp, service, level, request_id, trace_id,
// correlation_id, endpoint, duration, status.
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// Logger is an alias so callers can name the type without importing log/slog.
type Logger = *slog.Logger

type ctxKey int

const fieldsKey ctxKey = 0

// Fields carried on the request context and merged into every log line.
type Fields struct {
	RequestID     string
	TraceID       string
	CorrelationID string
	Endpoint      string
}

// Setup installs a JSON slog handler as the default logger.
// level is one of debug, info, warn, error (case-insensitive).
func Setup(service, level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Key = "timestamp"
			}
			if a.Key == slog.MessageKey {
				a.Key = "message"
			}
			return a
		},
	})
	l := slog.New(h).With("service", service)
	slog.SetDefault(l)
	return l
}

// WithFields returns a context carrying log fields for the current request.
func WithFields(ctx context.Context, f Fields) context.Context {
	return context.WithValue(ctx, fieldsKey, f)
}

// FromContext returns the log fields attached to ctx, or the zero value.
func FromContext(ctx context.Context) Fields {
	if f, ok := ctx.Value(fieldsKey).(Fields); ok {
		return f
	}
	return Fields{}
}

// L returns a logger pre-populated with the context's request fields.
func L(ctx context.Context) *slog.Logger {
	f := FromContext(ctx)
	l := slog.Default()
	if f.RequestID != "" {
		l = l.With("request_id", f.RequestID)
	}
	if f.TraceID != "" {
		l = l.With("trace_id", f.TraceID)
	}
	if f.CorrelationID != "" {
		l = l.With("correlation_id", f.CorrelationID)
	}
	if f.Endpoint != "" {
		l = l.With("endpoint", f.Endpoint)
	}
	return l
}
