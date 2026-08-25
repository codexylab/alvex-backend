package middleware

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

// responseWriter wraps http.ResponseWriter to capture the status code and
// implements http.Hijacker for WebSocket upgrades.
type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("underlying ResponseWriter does not implement http.Hijacker")
}

// RequestID injects a short unique request ID into the context and response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		id := fmt.Sprintf("%08x", b)
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID retrieves the request ID from the context.
func GetRequestID(r *http.Request) string {
	if id, ok := r.Context().Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// Logger is an HTTP middleware that logs every request with structured slog
// JSON output including method, path, status, latency, bytes, IP, and request ID.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start   := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		slog.Info("request",
			"id",       GetRequestID(r),
			"method",   r.Method,
			"path",     r.URL.Path,
			"status",   wrapped.status,
			"latency",  time.Since(start).String(),
			"bytes",    wrapped.bytes,
			"ip",       r.RemoteAddr,
		)
	})
}
