package middleware

import (
	"net/http"

	"github.com/codexylab/alvex-backend/pkg/response"
)

// LimitRequestBody enforces a global safety ceiling. Individual endpoints can
// apply smaller limits appropriate to their payload contract.
func LimitRequestBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				response.PayloadTooLarge(w, "Request body exceeds the maximum allowed size")
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
