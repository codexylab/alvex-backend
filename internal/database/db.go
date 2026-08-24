package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	_ "modernc.org/sqlite" // SQLite driver (dev/testing — no install required)
	_ "github.com/lib/pq"  // PostgreSQL driver (production)
)

// placeholderRe matches PostgreSQL-style positional placeholders like $1, $2, ...
var placeholderRe = regexp.MustCompile(`\$\d+`)

// DB wraps the standard sql.DB with ALVEX-specific helpers.
type DB struct {
	*sql.DB
	driver string // "sqlite" or "postgres"
}

// Connect opens a database connection based on the DATABASE_URL format.
//
// SQLite (development):
//   DATABASE_URL=sqlite://./alvex.db
//
// PostgreSQL (production):
//   DATABASE_URL=postgres://user:pass@host:5432/dbname?sslmode=disable
func Connect(databaseURL string) (*DB, error) {
	var driver, dsn string

	if strings.HasPrefix(databaseURL, "sqlite://") {
		driver = "sqlite"
		dsn = strings.TrimPrefix(databaseURL, "sqlite://")
		log.Println("📦 Using SQLite database (development mode)")
	} else {
		driver = "postgres"
		dsn = databaseURL
		log.Println("🐘 Using PostgreSQL database")
	}

	sqlDB, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	// Connection pool settings (smaller limits for SQLite)
	if driver == "postgres" {
		sqlDB.SetMaxOpenConns(25)
		sqlDB.SetMaxIdleConns(10)
	} else {
		sqlDB.SetMaxOpenConns(1) // SQLite is single-writer
	}

	log.Println("✅ Database connected successfully")
	return &DB{DB: sqlDB, driver: driver}, nil
}

// NewDB wraps a raw *sql.DB with the specified driver name ("sqlite" or "postgres").
func NewDB(sqlDB *sql.DB, driverName string) *DB {
	return &DB{DB: sqlDB, driver: driverName}
}

// RunMigrations creates all tables directly from embedded SQL.
// For SQLite we run inline SQL (no migrate tool needed).
// For PostgreSQL we also run inline SQL for simplicity.
func (db *DB) RunMigrations() error {
	schema := db.getSchema()
	
	// Execute each statement separately
	stmts := splitStatements(schema)
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			// Ignore "already exists" errors
			if !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("migration failed on statement [%.60s...]: %w", stmt, err)
			}
		}
	}

	log.Println("✅ Database schema ready")
	return nil
}

// RunColumnMigrations adds new columns to existing tables for database upgrades.
// Uses ALTER TABLE with graceful error handling so it is safe to run on both
// fresh and existing installations.
func (db *DB) RunColumnMigrations() error {
	type colMigration struct {
		table string
		col   string
		def   string
	}
	cols := []colMigration{
		{"activity_logs", "session_id",           "TEXT"},
		{"activity_logs", "latency_ms",           "INTEGER NOT NULL DEFAULT 0"},
		{"clients",       "portal_token",         "TEXT"},
		{"clients",       "custom_rate",          "DECIMAL(10,2)"},
		{"clients",       "gemini_api_key",       "TEXT"},
		{"clients",       "groq_api_key",         "TEXT"},
		{"clients",       "groq_fallback_enabled", "INTEGER NOT NULL DEFAULT 0"},
		{"clients",       "scraped_content",      "TEXT"},
		{"clients",       "scrape_synced_at",     "TIMESTAMPTZ"},
		{"clients",       "scrape_enabled",       "INTEGER NOT NULL DEFAULT 0"},
		{"clients",       "scrape_interval_hours","INTEGER NOT NULL DEFAULT 24"},
		{"clients",       "widget_chat_enabled",         "INTEGER NOT NULL DEFAULT 1"},
		{"clients",       "widget_ticketing_enabled",    "INTEGER NOT NULL DEFAULT 1"},
		{"clients",       "widget_admin_msg_enabled",    "INTEGER NOT NULL DEFAULT 1"},
		{"clients",       "widget_image_search_enabled", "INTEGER NOT NULL DEFAULT 1"},
		{"clients",       "widget_ticketing_allowed",    "INTEGER NOT NULL DEFAULT 1"},
		{"clients",       "widget_admin_msg_allowed",    "INTEGER NOT NULL DEFAULT 1"},
		{"clients",       "widget_image_search_allowed", "INTEGER NOT NULL DEFAULT 1"},
		{"clients",       "widget_brand_name",           "TEXT"},
		{"clients",       "widget_logo_url",             "TEXT"},
		{"clients",       "widget_primary_color",        "TEXT"},
		{"clients",       "widget_secondary_color",      "TEXT"},
		{"clients",       "widget_remove_branding",      "INTEGER NOT NULL DEFAULT 0"},
		{"clients",       "widget_branding_allowed",     "INTEGER NOT NULL DEFAULT 1"},
		{"clients",       "guardrails_enabled",          "INTEGER NOT NULL DEFAULT 0"},
		{"clients",       "guardrails_reply",            "TEXT"},
		{"clients",       "chat_retention_days",         "INTEGER NOT NULL DEFAULT 30"},
		{"activity_logs", "is_ticket",                  "INTEGER NOT NULL DEFAULT 0"},
		{"activity_logs", "reaction",                   "TEXT"},
		{"activity_logs", "image_data",                 "TEXT"},
		{"activity_logs", "needs_human",                "INTEGER NOT NULL DEFAULT 0"},
		{"activity_logs", "human_reply",                "TEXT"},
		{"activity_logs", "replied_at",                 "TIMESTAMPTZ"},
		{"activity_logs", "handoff_reason",             "TEXT"},
		{"clients",       "daily_msg_count",            "INTEGER NOT NULL DEFAULT 0"},
		{"clients",       "msg_count_date",             "TEXT"},
		{"clients",       "stripe_customer_id",         "TEXT"},
		{"clients",       "stripe_subscription_id",     "TEXT"},
		{"clients",       "onboarding_status",          "TEXT NOT NULL DEFAULT 'complete'"},
	}
	for _, c := range cols {
		stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", c.table, c.col, c.def)
		if _, err := db.Exec(stmt); err != nil {
			// Ignore "already exists" / "duplicate column" errors — column is already there.
			errMsg := strings.ToLower(err.Error())
			if strings.Contains(errMsg, "already") || strings.Contains(errMsg, "duplicate") {
				continue
			}
			return fmt.Errorf("column migration failed [%s.%s]: %w", c.table, c.col, err)
		}
	}
	log.Println("✅ Column migrations applied")
	return nil
}

// IsSQLite returns true if using SQLite.
func (db *DB) IsSQLite() bool {
	return db.driver == "sqlite"
}

// Adapt converts a PostgreSQL-style query (using $1, $2, ...) to SQLite style
// (using ? placeholders) when running in SQLite mode. In PostgreSQL mode it
// returns the query unchanged.
func (db *DB) Adapt(query string) string {
	if db.driver != "sqlite" {
		return query
	}
	return placeholderRe.ReplaceAllString(query, "?")
}

// getSchema returns the CREATE TABLE SQL compatible with both SQLite and PostgreSQL.
func (db *DB) getSchema() string {
	if db.driver == "sqlite" {
		return sqliteSchema
	}
	return postgresSchema
}

// splitStatements splits a SQL string by semicolons into individual statements.
func splitStatements(sql string) []string {
	return strings.Split(sql, ";")
}

// --- SQLite Schema (development) ---
const sqliteSchema = `
CREATE TABLE IF NOT EXISTS users (
    id         TEXT PRIMARY KEY,
    email      TEXT UNIQUE NOT NULL,
    name       TEXT NOT NULL DEFAULT '',
    role       TEXT NOT NULL DEFAULT 'admin',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS clients (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    domain           TEXT,
    status           TEXT NOT NULL DEFAULT 'Active',
    provider         TEXT NOT NULL DEFAULT 'Gemini',
    model            TEXT NOT NULL DEFAULT 'Gemini 2.0 Flash',
    api_key          TEXT,
    gemini_api_key   TEXT,
    groq_api_key     TEXT,
    groq_fallback_enabled INTEGER NOT NULL DEFAULT 0,
    system_persona   TEXT,
    webhook_url      TEXT,
    temperature      REAL NOT NULL DEFAULT 0.7,
    strict_adherence INTEGER NOT NULL DEFAULT 1,
    billing_plan     TEXT NOT NULL DEFAULT 'Basic',
    custom_rate      REAL,
    portal_token     TEXT,
    scraped_content  TEXT,
    scrape_synced_at DATETIME,
    scrape_enabled   INTEGER NOT NULL DEFAULT 0,
    scrape_interval_hours INTEGER NOT NULL DEFAULT 24,
    widget_chat_enabled   INTEGER NOT NULL DEFAULT 1,
    widget_ticketing_enabled INTEGER NOT NULL DEFAULT 1,
    widget_admin_msg_enabled INTEGER NOT NULL DEFAULT 1,
    widget_image_search_enabled INTEGER NOT NULL DEFAULT 1,
    widget_ticketing_allowed INTEGER NOT NULL DEFAULT 1,
    widget_admin_msg_allowed INTEGER NOT NULL DEFAULT 1,
    widget_image_search_allowed INTEGER NOT NULL DEFAULT 1,
    widget_brand_name   TEXT,
    widget_logo_url     TEXT,
    widget_primary_color TEXT,
    widget_secondary_color TEXT,
    widget_remove_branding INTEGER NOT NULL DEFAULT 0,
    widget_branding_allowed INTEGER NOT NULL DEFAULT 1,
    guardrails_enabled INTEGER NOT NULL DEFAULT 0,
    guardrails_reply   TEXT,
    chat_retention_days INTEGER NOT NULL DEFAULT 30,
    daily_msg_count  INTEGER NOT NULL DEFAULT 0,
    msg_count_date   TEXT,
    stripe_customer_id TEXT,
    stripe_subscription_id TEXT,
    onboarding_status TEXT NOT NULL DEFAULT 'complete',
    owner_id         TEXT REFERENCES users(id),
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS invoices (
    id          TEXT PRIMARY KEY,
    client_id   TEXT REFERENCES clients(id),
    client_name TEXT NOT NULL,
    amount      REAL NOT NULL,
    status      TEXT NOT NULL DEFAULT 'Pending',
    due_date    TEXT,
    paid_at     DATETIME,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS activity_logs (
    id          TEXT PRIMARY KEY,
    client_id   TEXT REFERENCES clients(id),
    client_name TEXT NOT NULL,
    channel     TEXT NOT NULL DEFAULT 'web',
    user_ref    TEXT,
    session_id  TEXT,
    message     TEXT,
    ai_response TEXT,
    status      TEXT NOT NULL DEFAULT 'Handling...',
    latency_ms  INTEGER NOT NULL DEFAULT 0,
    is_ticket   INTEGER NOT NULL DEFAULT 0,
    reaction    TEXT,
    image_data  TEXT,
    needs_human INTEGER NOT NULL DEFAULT 0,
    human_reply TEXT,
    replied_at  DATETIME,
    handoff_reason TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS faqs (
    id         TEXT PRIMARY KEY,
    client_id  TEXT REFERENCES clients(id) ON DELETE CASCADE,
    question   TEXT NOT NULL,
    answer     TEXT NOT NULL,
    is_approved INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS document_chunks (
    id          TEXT PRIMARY KEY,
    client_id   TEXT REFERENCES clients(id) ON DELETE CASCADE,
    content     TEXT NOT NULL,
    embedding   TEXT NOT NULL,
    source_url  TEXT NOT NULL DEFAULT '',
    chunk_index INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS documents (
    id          TEXT PRIMARY KEY,
    client_id   TEXT REFERENCES clients(id) ON DELETE CASCADE,
    filename    TEXT NOT NULL,
    file_type   TEXT NOT NULL,
    file_size   INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'processed',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS leads (
    id          TEXT PRIMARY KEY,
    client_id   TEXT REFERENCES clients(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    email       TEXT,
    phone       TEXT,
    session_id  TEXT,
    source      TEXT NOT NULL DEFAULT 'widget',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS key_audit_logs (
    id          TEXT PRIMARY KEY,
    client_id   TEXT REFERENCES clients(id) ON DELETE CASCADE,
    key_type    TEXT NOT NULL,
    rotated_by  TEXT NOT NULL DEFAULT 'admin',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_clients_status   ON clients(status);
CREATE INDEX IF NOT EXISTS idx_invoices_status  ON invoices(status);
CREATE INDEX IF NOT EXISTS idx_activity_created ON activity_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_faqs_client      ON faqs(client_id);
CREATE INDEX IF NOT EXISTS idx_chunks_client    ON document_chunks(client_id);
CREATE INDEX IF NOT EXISTS idx_documents_client ON documents(client_id);
CREATE INDEX IF NOT EXISTS idx_leads_client     ON leads(client_id);
`

// --- PostgreSQL Schema (production) ---
const postgresSchema = `
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS users (
    id         TEXT PRIMARY KEY,
    email      VARCHAR(255) UNIQUE NOT NULL,
    name       VARCHAR(200) NOT NULL DEFAULT '',
    role       VARCHAR(20)  NOT NULL DEFAULT 'admin',
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS clients (
    id               VARCHAR(100) PRIMARY KEY,
    name             VARCHAR(255) NOT NULL,
    domain           VARCHAR(255),
    status           VARCHAR(20)  NOT NULL DEFAULT 'Active',
    provider         VARCHAR(50)  NOT NULL DEFAULT 'Gemini',
    model            VARCHAR(100) NOT NULL DEFAULT 'Gemini 2.0 Flash',
    api_key          TEXT,
    gemini_api_key   TEXT,
    groq_api_key     TEXT,
    groq_fallback_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    system_persona   TEXT,
    webhook_url      VARCHAR(500),
    temperature      DECIMAL(3,1) NOT NULL DEFAULT 0.7,
    strict_adherence BOOLEAN      NOT NULL DEFAULT TRUE,
    billing_plan     VARCHAR(50)  NOT NULL DEFAULT 'Basic',
    custom_rate      DECIMAL(10,2),
    portal_token     VARCHAR(64),
    scraped_content  TEXT,
    scrape_synced_at TIMESTAMPTZ,
    scrape_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    scrape_interval_hours INTEGER NOT NULL DEFAULT 24,
    widget_chat_enabled   BOOLEAN NOT NULL DEFAULT TRUE,
    widget_ticketing_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    widget_admin_msg_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    widget_image_search_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    widget_ticketing_allowed BOOLEAN NOT NULL DEFAULT TRUE,
    widget_admin_msg_allowed BOOLEAN NOT NULL DEFAULT TRUE,
    widget_image_search_allowed BOOLEAN NOT NULL DEFAULT TRUE,
    widget_brand_name   TEXT,
    widget_logo_url     TEXT,
    widget_primary_color VARCHAR(50),
    widget_secondary_color VARCHAR(50),
    widget_remove_branding BOOLEAN NOT NULL DEFAULT FALSE,
    widget_branding_allowed BOOLEAN NOT NULL DEFAULT TRUE,
    guardrails_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    guardrails_reply   TEXT,
    chat_retention_days INTEGER NOT NULL DEFAULT 30,
    daily_msg_count  INTEGER NOT NULL DEFAULT 0,
    msg_count_date   VARCHAR(20),
    stripe_customer_id VARCHAR(100),
    stripe_subscription_id VARCHAR(100),
    onboarding_status VARCHAR(50) NOT NULL DEFAULT 'complete',
    owner_id         TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS invoices (
    id          VARCHAR(50)   PRIMARY KEY,
    client_id   VARCHAR(100)  REFERENCES clients(id) ON DELETE SET NULL,
    client_name VARCHAR(255)  NOT NULL,
    amount      DECIMAL(10,2) NOT NULL,
    status      VARCHAR(20)   NOT NULL DEFAULT 'Pending',
    due_date    DATE,
    paid_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS activity_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id   VARCHAR(100) REFERENCES clients(id) ON DELETE SET NULL,
    client_name VARCHAR(255) NOT NULL,
    channel     VARCHAR(20)  NOT NULL DEFAULT 'web',
    user_ref    VARCHAR(100),
    session_id  VARCHAR(100),
    message     TEXT,
    ai_response TEXT,
    status      VARCHAR(30)  NOT NULL DEFAULT 'Handling...',
    latency_ms  INTEGER      NOT NULL DEFAULT 0,
    is_ticket   BOOLEAN      NOT NULL DEFAULT FALSE,
    reaction    TEXT,
    image_data  TEXT,
    needs_human BOOLEAN      NOT NULL DEFAULT FALSE,
    human_reply TEXT,
    replied_at  TIMESTAMPTZ,
    handoff_reason TEXT,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS faqs (
    id         VARCHAR(100) PRIMARY KEY,
    client_id  VARCHAR(100) REFERENCES clients(id) ON DELETE CASCADE,
    question   TEXT NOT NULL,
    answer     TEXT NOT NULL,
    is_approved BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS document_chunks (
    id          VARCHAR(100) PRIMARY KEY,
    client_id   VARCHAR(100) REFERENCES clients(id) ON DELETE CASCADE,
    content     TEXT NOT NULL,
    embedding   TEXT NOT NULL,
    source_url  VARCHAR(500) NOT NULL DEFAULT '',
    chunk_index INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS documents (
    id          VARCHAR(100) PRIMARY KEY,
    client_id   VARCHAR(100) REFERENCES clients(id) ON DELETE CASCADE,
    filename    VARCHAR(255) NOT NULL,
    file_type   VARCHAR(50)  NOT NULL,
    file_size   BIGINT       NOT NULL DEFAULT 0,
    status      VARCHAR(50)  NOT NULL DEFAULT 'processed',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS leads (
    id          VARCHAR(100) PRIMARY KEY,
    client_id   VARCHAR(100) REFERENCES clients(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    email       VARCHAR(255),
    phone       VARCHAR(50),
    session_id  VARCHAR(100),
    source      VARCHAR(50)  NOT NULL DEFAULT 'widget',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS key_audit_logs (
    id          VARCHAR(100) PRIMARY KEY,
    client_id   VARCHAR(100) REFERENCES clients(id) ON DELETE CASCADE,
    key_type    VARCHAR(50)  NOT NULL,
    rotated_by  VARCHAR(100) NOT NULL DEFAULT 'admin',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_clients_status   ON clients(status);
CREATE INDEX IF NOT EXISTS idx_invoices_status  ON invoices(status);
CREATE INDEX IF NOT EXISTS idx_activity_created ON activity_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_faqs_client      ON faqs(client_id);
CREATE INDEX IF NOT EXISTS idx_chunks_client    ON document_chunks(client_id);
CREATE INDEX IF NOT EXISTS idx_documents_client ON documents(client_id);
CREATE INDEX IF NOT EXISTS idx_leads_client     ON leads(client_id);
`

// DBFilePath returns the SQLite file path from DATABASE_URL.
func DBFilePath(databaseURL string) string {
	if strings.HasPrefix(databaseURL, "sqlite://") {
		return strings.TrimPrefix(databaseURL, "sqlite://")
	}
	return ""
}

// EnsureDBDir creates the directory for the SQLite file if needed.
func EnsureDBDir(databaseURL string) {
	path := DBFilePath(databaseURL)
	if path == "" {
		return
	}
	dir := path[:strings.LastIndex(path, "/")]
	if dir != "" && dir != "." {
		os.MkdirAll(dir, 0755)
	}
}
