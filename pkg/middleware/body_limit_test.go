package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLimitRequestBodyRejectsKnownOversizedPayload(t *testing.T) {
	called := false
	handler := LimitRequestBody(4)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("12345"))

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", recorder.Code)
	}
	if called {
		t.Fatal("oversized payload reached the endpoint")
	}
}
