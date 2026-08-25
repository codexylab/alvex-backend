package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/services"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Create tables
	schema := `
	CREATE TABLE users (
		id         TEXT PRIMARY KEY,
		email      TEXT UNIQUE NOT NULL,
		name       TEXT NOT NULL DEFAULT '',
		role       TEXT NOT NULL DEFAULT 'admin',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE clients (
		id                   TEXT PRIMARY KEY,
		name                 TEXT NOT NULL,
		domain               TEXT,
		status               TEXT NOT NULL DEFAULT 'Active',
		provider             TEXT NOT NULL DEFAULT 'Gemini',
		model                TEXT NOT NULL DEFAULT 'Gemini 2.0 Flash',
		api_key              TEXT,
		gemini_api_key       TEXT,
		groq_api_key         TEXT,
		groq_fallback_enabled INTEGER NOT NULL DEFAULT 0,
		system_persona       TEXT,
		webhook_url          TEXT,
		temperature          REAL NOT NULL DEFAULT 0.7,
		strict_adherence     INTEGER NOT NULL DEFAULT 1,
		billing_plan         TEXT NOT NULL DEFAULT 'Basic',
		custom_rate          REAL,
		portal_token         TEXT,
		scraped_content      TEXT,
		scrape_synced_at     DATETIME,
		scrape_enabled       INTEGER NOT NULL DEFAULT 0,
		scrape_interval_hours INTEGER NOT NULL DEFAULT 24,
		widget_chat_enabled   INTEGER NOT NULL DEFAULT 1,
		widget_ticketing_enabled INTEGER NOT NULL DEFAULT 1,
		widget_admin_msg_enabled INTEGER NOT NULL DEFAULT 1,
		widget_image_search_enabled INTEGER NOT NULL DEFAULT 1,
		widget_ticketing_allowed INTEGER NOT NULL DEFAULT 1,
		widget_admin_msg_allowed INTEGER NOT NULL DEFAULT 1,
		widget_image_search_allowed INTEGER NOT NULL DEFAULT 1,
		widget_brand_name     TEXT,
		widget_logo_url       TEXT,
		widget_primary_color  TEXT,
		widget_secondary_color TEXT,
		widget_remove_branding INTEGER NOT NULL DEFAULT 0,
		widget_branding_allowed INTEGER NOT NULL DEFAULT 1,
		guardrails_enabled   INTEGER NOT NULL DEFAULT 0,
		guardrails_reply     TEXT,
		chat_retention_days  INTEGER NOT NULL DEFAULT 30,
		owner_id             TEXT REFERENCES users(id),
		created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err = db.Exec(schema)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Insert test user
	_, err = db.Exec(`INSERT INTO users (id, email, name) VALUES ('dev-user-001', 'dev@alvex.ai', 'Dev Admin')`)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}

	return db
}

func TestClientHandler_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	wrapDB := database.NewDB(db, "sqlite")
	repo := repository.NewSQLClientRepository(wrapDB)
	svc := services.NewClientService(repo, "")
	h := &ClientHandler{Service: svc}

	// Prepare request
	reqBody := `{"name":"Nexus Dynamics","domain":"nexus-dyn.ai","provider":"Gemini","model":"Gemini 2.0 Flash","billing_plan":"Enterprise"}`
	req := httptest.NewRequest("POST", "/api/v1/clients", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Inject test user into context (simulating auth middleware)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, "dev-user-001")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status 201 Created, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	// Unmarshal models.Envelope first
	type envelope struct {
		Data models.Client `json:"data"`
	}
	var res envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if res.Data.Name != "Nexus Dynamics" {
		t.Errorf("expected client name 'Nexus Dynamics', got '%s'", res.Data.Name)
	}
	if res.Data.ID != "nexus-dynamics" {
		t.Errorf("expected client ID 'nexus-dynamics', got '%s'", res.Data.ID)
	}
}

func TestClientHandler_List(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed client
	_, err := db.Exec(`
		INSERT INTO clients (id, name, domain, provider, model, api_key, system_persona, webhook_url, temperature, strict_adherence, billing_plan, owner_id)
		VALUES ('nexus-dynamics', 'Nexus Dynamics', 'nexus-dyn.ai', 'Gemini', 'Gemini 2.0 Flash', 'ALVX-NEXD-8921', 'persona', 'webhook', 0.7, 1, 'Enterprise', 'dev-user-001')
	`)
	if err != nil {
		t.Fatalf("failed to seed client: %v", err)
	}

	wrapDB := database.NewDB(db, "sqlite")
	repo := repository.NewSQLClientRepository(wrapDB)
	svc := services.NewClientService(repo, "")
	h := &ClientHandler{Service: svc}

	req := httptest.NewRequest("GET", "/api/v1/clients?search=nexus", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", rec.Code)
	}

	// Wrapper struct for response list
	type envelope struct {
		Data struct {
			Data []models.Client `json:"data"`
		} `json:"data"`
	}
	var res envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal response: %v. Body: %s", err, rec.Body.String())
	}

	if len(res.Data.Data) != 1 {
		t.Errorf("expected 1 client in response, got %d", len(res.Data.Data))
	}
}
