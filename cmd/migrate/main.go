package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/codexylab/alvex-backend/pkg/config"
	"github.com/codexylab/alvex-backend/pkg/database"
)

func main() {
	cfg := config.Load()
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		slog.Error("migration database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if db.IsSQLite() {
		if err := db.RunMigrations(); err == nil {
			err = db.RunColumnMigrations()
		}
	} else {
		err = db.MigratePostgres(ctx)
	}
	if err != nil {
		slog.Error("database migration failed", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrations are up to date")
}
