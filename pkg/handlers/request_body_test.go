package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadRequestBodyRejectsOversizedPayload(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader([]byte("12345")))
	recorder := httptest.NewRecorder()

	_, ok := readRequestBody(recorder, request, 4)

	if ok {
		t.Fatal("expected oversized payload to be rejected")
	}
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected %d, got %d", http.StatusRequestEntityTooLarge, recorder.Code)
	}
}
