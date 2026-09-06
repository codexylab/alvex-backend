package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/tenant"
)

type auditRepositorySpy struct {
	entries []repository.AuditEntry
}

func (s *auditRepositorySpy) InsertAuditEntry(_ context.Context, entry repository.AuditEntry) error {
	s.entries = append(s.entries, entry)
	return nil
}

func TestAuditMutationsRecordsSuccessfulTenantWrite(t *testing.T) {
	repo := &auditRepositorySpy{}
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := tenant.WithScope(r.Context(), tenant.Scope{
				OrganizationID: "org_one",
				UserID:         "user_one",
				Role:           "owner",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	router.Use(AuditMutations(repo))
	router.Post("/api/v1/clients/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/clients/client_one", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if len(repo.entries) != 1 {
		t.Fatalf("expected one audit entry, got %d", len(repo.entries))
	}
	entry := repo.entries[0]
	if entry.OrganizationID != "org_one" || entry.ActorUserID != "user_one" {
		t.Fatalf("unexpected actor scope: %#v", entry)
	}
	if entry.Action != "POST /api/v1/clients/{id}" || entry.ResourceID != "client_one" {
		t.Fatalf("unexpected action details: %#v", entry)
	}
	if entry.IPAddress != "192.0.2.10" {
		t.Fatalf("unexpected IP address: %q", entry.IPAddress)
	}
}

func TestAuditMutationsSkipsFailedWrite(t *testing.T) {
	repo := &auditRepositorySpy{}
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(tenant.WithScope(r.Context(), tenant.Scope{
				OrganizationID: "org_one",
				UserID:         "user_one",
			})))
		})
	})
	router.Use(AuditMutations(repo))
	router.Delete("/api/v1/clients/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/clients/client_one", nil))

	if len(repo.entries) != 0 {
		t.Fatalf("expected no audit entry for failed mutation, got %d", len(repo.entries))
	}
}
