package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticatorFailsClosedWithoutConfiguration(t *testing.T) {
	authenticator := NewAuthenticator("", "", "production", "", nil)
	handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clients", nil)
	req.Header.Set("Authorization", "Bearer arbitrary-token")

	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, req)

	if responseRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, responseRecorder.Code)
	}
}

func TestAuthenticatorAllowsExplicitDevelopmentToken(t *testing.T) {
	authenticator := NewAuthenticator("", "", "development", "local-test-token", nil)
	handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := GetUserID(r); got != "dev-user-001" {
			t.Fatalf("expected development user ID, got %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clients", nil)
	req.Header.Set("Authorization", "Bearer local-test-token")

	responseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(responseRecorder, req)

	if responseRecorder.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, responseRecorder.Code)
	}
}

func TestBearerQueryTokenIsLimitedToActivityWebSocket(t *testing.T) {
	apiRequest := httptest.NewRequest(http.MethodGet, "/api/v1/clients?token=secret", nil)
	if token := extractBearerToken(apiRequest); token != "" {
		t.Fatalf("expected API query token to be rejected, got %q", token)
	}

	websocketRequest := httptest.NewRequest(http.MethodGet, "/ws/activity?token=secret", nil)
	if token := extractBearerToken(websocketRequest); token != "secret" {
		t.Fatalf("expected WebSocket query token, got %q", token)
	}
}
