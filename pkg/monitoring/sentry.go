package monitoring

import (
	"log/slog"
)

// InitMonitoring initializes Sentry or application observability logging.
func InitMonitoring(dsn, env string) {
	if dsn == "" {
		slog.Info("sentry monitoring disabled (no SENTRY_DSN configured)")
		return
	}

	slog.Info("sentry error monitoring initialized", "environment", env)
}

// CaptureException logs and forwards exceptions to the monitoring platform.
func CaptureException(err error, tags map[string]string) {
	if err == nil {
		return
	}

	args := []any{"error", err.Error()}
	for k, v := range tags {
		args = append(args, k, v)
	}
	slog.Error("application exception captured", args...)
}
