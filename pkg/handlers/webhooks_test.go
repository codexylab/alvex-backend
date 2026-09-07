package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestReceiveWhatsAppRejectsMissingSignatureWithUnauthorized(t *testing.T) {
	handler := &WebhookHandler{WhatsAppAppSecret: "app-secret"}
	router := chi.NewRouter()
	router.Post("/webhook/wa/v2/{clientId}", handler.ReceiveWhatsApp)
	request := httptest.NewRequest(
		http.MethodPost,
		"/webhook/wa/v2/client_one",
		strings.NewReader(`{"object":"whatsapp_business_account"}`),
	)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Invalid WhatsApp webhook signature") {
		t.Fatalf("unexpected response %s", recorder.Body.String())
	}
}
