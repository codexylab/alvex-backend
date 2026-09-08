package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/services"
	"github.com/codexylab/alvex-backend/pkg/tenant"
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
		organization_id      TEXT NOT NULL DEFAULT 'org_test',
		name                 TEXT NOT NULL,
		domain               TEXT,
		allowed_origins      TEXT NOT NULL DEFAULT '[]',
		whatsapp_phone_number_id TEXT,
		status               TEXT NOT NULL DEFAULT 'Active',
		provider             TEXT NOT NULL DEFAULT 'Gemini',
		model                TEXT NOT NULL DEFAULT 'Gemini 2.0 Flash',
		api_key              TEXT,
		openai_api_key       TEXT,
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
	CREATE TABLE client_memberships (
		client_id  TEXT NOT NULL,
		user_id    TEXT NOT NULL,
		role       TEXT NOT NULL,
		status     TEXT NOT NULL,
		PRIMARY KEY (client_id, user_id)
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
	repo := repository.NewTenantSQLClientRepository(wrapDB)
	svc := services.NewClientService(repo, "", "http://localhost:8080")
	h := &ClientHandler{Service: svc}

	// Prepare request
	reqBody := `{"name":"Nexus Dynamics","domain":"nexus-dyn.ai","provider":"Gemini","model":"Gemini 2.0 Flash","billing_plan":"Enterprise"}`
	req := httptest.NewRequest("POST", "/api/v1/clients", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Inject test user into context (simulating auth middleware)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, "dev-user-001")
	ctx = tenant.WithScope(ctx, tenant.Scope{OrganizationID: "org_test", UserID: "dev-user-001", Role: "owner"})
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
	if !regexp.MustCompile(`^nexus-dynamics-[0-9a-f]{16}$`).MatchString(res.Data.ID) {
		t.Errorf("expected globally unique client ID, got '%s'", res.Data.ID)
	}
}

func TestClientHandler_CreateAllowsSameNameAcrossOrganizations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO clients (id, organization_id, name, domain, owner_id)
		VALUES ('junaid', 'org_other', 'junaid', 'https://junaid.example', 'dev-user-001')
	`)
	if err != nil {
		t.Fatalf("failed to seed other tenant client: %v", err)
	}

	wrapDB := database.NewDB(db, "sqlite")
	repo := repository.NewTenantSQLClientRepository(wrapDB)
	handler := &ClientHandler{Service: services.NewClientService(repo, "", "http://localhost:8080")}
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/clients",
		strings.NewReader(`{"name":"junaid","domain":"junaid.com","provider":"Gemini","model":"Gemini 2.5 Flash","billing_plan":"Basic"}`),
	)
	request = request.WithContext(tenant.WithScope(request.Context(), tenant.Scope{
		OrganizationID: "org_test",
		UserID:         "dev-user-001",
		Role:           "owner",
	}))
	request = request.WithContext(context.WithValue(request.Context(), middleware.UserIDKey, "dev-user-001"))
	responseRecorder := httptest.NewRecorder()

	handler.Create(responseRecorder, request)
	if responseRecorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", responseRecorder.Code, responseRecorder.Body.String())
	}

	var createdCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM clients WHERE organization_id = 'org_test' AND name = 'junaid'`).Scan(&createdCount); err != nil {
		t.Fatalf("count created client: %v", err)
	}
	if createdCount != 1 {
		t.Fatalf("expected one junaid client in org_test, got %d", createdCount)
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
	repo := repository.NewTenantSQLClientRepository(wrapDB)
	svc := services.NewClientService(repo, "", "http://localhost:8080")
	h := &ClientHandler{Service: svc}

	req := httptest.NewRequest("GET", "/api/v1/clients?search=nexus", nil)
	req = req.WithContext(tenant.WithScope(req.Context(), tenant.Scope{OrganizationID: "org_test"}))
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

func TestClientHandler_SetStatusUsesDesiredState(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO clients (
			id, name, domain, status, api_key, system_persona, webhook_url, owner_id
		)
		VALUES (
			'status-client', 'Status Client', 'https://status.example.com', 'Active',
			'test-key', 'Test persona', 'https://api.example.com/webhook', 'dev-user-001'
		)
	`)
	if err != nil {
		t.Fatalf("failed to seed client: %v", err)
	}

	wrapDB := database.NewDB(db, "sqlite")
	repo := repository.NewTenantSQLClientRepository(wrapDB)
	handler := &ClientHandler{Service: services.NewClientService(repo, "", "http://localhost:8080")}
	router := chi.NewRouter()
	router.Patch("/clients/{id}/status", handler.ToggleStatus)

	for _, desiredStatus := range []string{"Active", "Suspended", "Suspended"} {
		request := httptest.NewRequest(
			http.MethodPatch,
			"/clients/status-client/status",
			strings.NewReader(`{"status":"`+desiredStatus+`"}`),
		)
		request = request.WithContext(tenant.WithScope(request.Context(), tenant.Scope{
			OrganizationID: "org_test",
		}))
		responseRecorder := httptest.NewRecorder()

		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusOK {
			t.Fatalf("set %s: expected 200, got %d: %s", desiredStatus, responseRecorder.Code, responseRecorder.Body.String())
		}

		var actualStatus string
		if err := db.QueryRow(`SELECT status FROM clients WHERE id = 'status-client'`).Scan(&actualStatus); err != nil {
			t.Fatalf("read status: %v", err)
		}
		if actualStatus != desiredStatus {
			t.Fatalf("expected %s, got %s", desiredStatus, actualStatus)
		}
	}
}

func TestClientHandler_SetAllStatusesIsTenantScoped(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO clients (id, organization_id, name, status, owner_id) VALUES
			('tenant-client-a', 'org_test', 'Tenant A', 'Active', 'dev-user-001'),
			('tenant-client-b', 'org_test', 'Tenant B', 'Suspended', 'dev-user-001'),
			('other-client', 'org_other', 'Other Tenant', 'Active', 'dev-user-001')
	`)
	if err != nil {
		t.Fatalf("failed to seed clients: %v", err)
	}

	wrapDB := database.NewDB(db, "sqlite")
	repo := repository.NewTenantSQLClientRepository(wrapDB)
	handler := &ClientHandler{Service: services.NewClientService(repo, "", "http://localhost:8080")}
	request := httptest.NewRequest(http.MethodPatch, "/clients/status", strings.NewReader(`{"status":"Suspended"}`))
	request = request.WithContext(tenant.WithScope(request.Context(), tenant.Scope{OrganizationID: "org_test"}))
	responseRecorder := httptest.NewRecorder()

	handler.SetAllStatuses(responseRecorder, request)
	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", responseRecorder.Code, responseRecorder.Body.String())
	}

	rows, err := db.Query(`SELECT id, status FROM clients ORDER BY id`)
	if err != nil {
		t.Fatalf("query clients: %v", err)
	}
	defer rows.Close()

	statuses := map[string]string{}
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			t.Fatalf("scan status: %v", err)
		}
		statuses[id] = status
	}
	if statuses["tenant-client-a"] != "Suspended" || statuses["tenant-client-b"] != "Suspended" {
		t.Fatalf("tenant clients were not suspended: %#v", statuses)
	}
	if statuses["other-client"] != "Active" {
		t.Fatalf("other tenant was modified: %#v", statuses)
	}
}

func TestClientHandler_GetOneRedactsStoredSecrets(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO clients (
			id, name, domain, openai_api_key, gemini_api_key, groq_api_key,
			system_persona, webhook_url, owner_id
		)
		VALUES (
			'secret-client', 'Secret Client', 'https://secret.example.com',
			'openai-secret', 'gemini-secret', 'groq-secret',
			'Test persona', 'https://api.example.com/webhook', 'dev-user-001'
		)
	`)
	if err != nil {
		t.Fatalf("failed to seed client: %v", err)
	}

	wrapDB := database.NewDB(db, "sqlite")
	repo := repository.NewTenantSQLClientRepository(wrapDB)
	handler := &ClientHandler{Service: services.NewClientService(
		repo,
		"12345678901234567890123456789012",
		"http://localhost:8080",
	)}
	router := chi.NewRouter()
	router.Get("/clients/{id}", handler.GetOne)
	request := httptest.NewRequest(http.MethodGet, "/clients/secret-client", nil)
	request = request.WithContext(tenant.WithScope(request.Context(), tenant.Scope{OrganizationID: "org_test"}))
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)
	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", responseRecorder.Code, responseRecorder.Body.String())
	}
	var envelope struct {
		Data models.Client `json:"data"`
	}
	if err := json.Unmarshal(responseRecorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Data.OpenAIAPIKey != "" || envelope.Data.GeminiAPIKey != "" || envelope.Data.GroqAPIKey != "" {
		t.Fatalf("response exposed secrets: %#v", envelope.Data)
	}
	var legacyClientKey sql.NullString
	var rotatedOpenAIKey, rotatedGeminiKey, rotatedGroqKey string
	if err := db.QueryRow(`
		SELECT api_key, openai_api_key, gemini_api_key, groq_api_key
		FROM clients WHERE id = 'secret-client'
	`).Scan(&legacyClientKey, &rotatedOpenAIKey, &rotatedGeminiKey, &rotatedGroqKey); err != nil {
		t.Fatalf("load rotated secrets: %v", err)
	}
	if legacyClientKey.Valid {
		t.Fatal("retired legacy client API key must remain unset")
	}
	for name, value := range map[string]string{
		"openai": rotatedOpenAIKey,
		"gemini": rotatedGeminiKey,
		"groq":   rotatedGroqKey,
	} {
		if !strings.HasPrefix(value, "v1:") {
			t.Errorf("%s secret was not upgraded to versioned encryption", name)
		}
	}
}
