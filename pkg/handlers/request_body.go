package handlers

import (
	"errors"
	"io"
	"net/http"

	"github.com/codexylab/alvex-backend/pkg/response"
)

const signedWebhookBodyLimit int64 = 1 << 20 // 1 MiB

// readRequestBody reads a bounded body and emits a consistent client error.
func readRequestBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	body, err := io.ReadAll(r.Body)
	if err == nil {
		return body, true
	}

	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		response.PayloadTooLarge(w, "Request body exceeds the allowed size")
		return nil, false
	}
	response.BadRequest(w, "Unable to read request body")
	return nil, false
}
