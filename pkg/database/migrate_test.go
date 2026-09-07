package database

import "testing"

func TestMigrationVersion(t *testing.T) {
	version, err := migrationVersion("migrations/002_expand_tenancy.up.sql")
	if err != nil {
		t.Fatalf("migrationVersion returned error: %v", err)
	}
	if version != 2 {
		t.Fatalf("expected version 2, got %d", version)
	}
}

func TestMigrationVersionRejectsInvalidName(t *testing.T) {
	if _, err := migrationVersion("migrations/invalid.sql"); err == nil {
		t.Fatal("expected invalid migration name to fail")
	}
}
