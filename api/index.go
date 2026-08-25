package handler

import (
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/codexylab/alvex-backend/pkg/config"
	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/monitoring"
	"github.com/codexylab/alvex-backend/pkg/queue"
	"github.com/codexylab/alvex-backend/pkg/router"
)

var (
	appHandler http.Handler
	initOnce   sync.Once
)

func initialize() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg := config.Load()
	monitoring.InitMonitoring(cfg.SentryDSN, cfg.Env)

	database.EnsureDBDir(cfg.DatabaseURL)
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed in vercel function", "error", err)
	} else {
		_ = db.RunMigrations()
		_ = db.RunColumnMigrations()
	}

	workerPool := queue.NewWorkerPool(2, 50)
	workerPool.Start()

	appHandler = router.New(cfg, db, workerPool)
}

// Handler is the Vercel serverless entrypoint
func Handler(w http.ResponseWriter, r *http.Request) {
	initOnce.Do(initialize)
	if appHandler != nil {
		appHandler.ServeHTTP(w, r)
	} else {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
	}
}
