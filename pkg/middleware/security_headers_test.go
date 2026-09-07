package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	expected := map[string]string{
		"Content-Security-Policy":   "default-src 'none'; frame-ancestors 'none'",
		"Permissions-Policy":        "camera=(), microphone=(), geolocation=(), payment=()",
		"Referrer-Policy":           "no-referrer",
		"Strict-Transport-Security": "max-age=63072000; includeSubDomains; preload",
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
	}
	for name, value := range expected {
		if actual := recorder.Header().Get(name); actual != value {
			t.Errorf("%s: expected %q, got %q", name, value, actual)
		}
	}
}
