package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/repository"
)

type widgetSessionResolverStub struct {
	session *repository.WidgetSession
	err     error
}

func (s widgetSessionResolverStub) Resolve(context.Context, string, string, string) (*repository.WidgetSession, error) {
	return s.session, s.err
}

func TestRequireWidgetSessionAttachesTrustedSession(t *testing.T) {
	resolver := widgetSessionResolverStub{session: &repository.WidgetSession{ID: "session_one", ClientID: "client_one"}}
	router := chi.NewRouter()
	router.With(RequireWidgetSession(resolver)).Get("/widget/v1/{clientId}/history", func(w http.ResponseWriter, r *http.Request) {
		session, ok := GetWidgetSession(r)
		if !ok || session.ID != "session_one" {
			t.Fatal("expected widget session in request context")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/widget/v1/client_one/history", nil)
	request.Header.Set("Origin", "https://example.com")
	request.Header.Set("Authorization", "Bearer valid-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, recorder.Code)
	}
}

func TestRequireWidgetSessionRejectsMissingOrInvalidCredentials(t *testing.T) {
	tests := []struct {
		name      string
		origin    string
		authorize string
		resolver  widgetSessionResolverStub
	}{
		{name: "missing origin", authorize: "Bearer token"},
		{name: "missing token", origin: "https://example.com"},
		{name: "invalid token", origin: "https://example.com", authorize: "Bearer token", resolver: widgetSessionResolverStub{err: errors.New("invalid")}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := chi.NewRouter()
			router.With(RequireWidgetSession(test.resolver)).Get("/widget/v1/{clientId}/history", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, "/widget/v1/client_one/history", nil)
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Authorization", test.authorize)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("expected %d, got %d", http.StatusUnauthorized, recorder.Code)
			}
		})
	}
}
