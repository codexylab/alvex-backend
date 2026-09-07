package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders adds browser hardening headers to every API response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")

		forwardedProto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
		if r.TLS != nil || strings.EqualFold(forwardedProto, "https") {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}

		next.ServeHTTP(w, r)
	})
}
