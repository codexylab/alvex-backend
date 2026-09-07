package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSupabaseAdminClientSendsServerSideInvitation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/invite" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret-key" || r.Header.Get("apikey") != "secret-key" {
			t.Fatal("missing Supabase admin credentials")
		}
		if got := r.URL.Query().Get("redirect_to"); got != "https://app.example.com/auth/callback?next=/set-password" {
			t.Fatalf("unexpected redirect URL %q", got)
		}
		var body struct {
			Email string            `json:"email"`
			Data  map[string]string `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode invitation request: %v", err)
		}
		if body.Email != "owner@example.com" || body.Data["full_name"] != "Owner" {
			t.Fatalf("unexpected invitation body: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"supabase-user","email":"owner@example.com"}`))
	}))
	defer server.Close()

	identity, err := NewSupabaseAdminClient(server.URL, "secret-key").InviteUser(
		context.Background(),
		"owner@example.com",
		"Owner",
		"https://app.example.com/auth/callback?next=/set-password",
	)
	if err != nil {
		t.Fatalf("invite user: %v", err)
	}
	if identity.ID != "supabase-user" || identity.Email != "owner@example.com" {
		t.Fatalf("unexpected identity: %#v", identity)
	}
}

func TestSupabaseAdminClientDoesNotExposeSecretInProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"secret-key"}`, http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	_, err := NewSupabaseAdminClient(server.URL, "secret-key").InviteUser(
		context.Background(), "owner@example.com", "Owner", "",
	)
	if err == nil || err.Error() != "Supabase invitation failed with status 422" {
		t.Fatalf("unexpected sanitized error: %v", err)
	}
}
