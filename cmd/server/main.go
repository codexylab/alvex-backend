package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/codexylab/alvex-backend/pkg/config"
	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/monitoring"
	"github.com/codexylab/alvex-backend/pkg/queue"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/router"
	"github.com/codexylab/alvex-backend/pkg/services"
)

func main() {
	// Configure structured JSON logging for production-grade observability.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
	log.SetFlags(0) // disable default log prefix; slog handles timestamps

	slog.Info("starting ALVEX backend server")

	// Load configuration from .env or environment variables
	cfg := config.Load()

	// Initialize Sentry error monitoring
	monitoring.InitMonitoring(cfg.SentryDSN, cfg.Env)

	// Ensure the SQLite directory exists (no-op for PostgreSQL)
	database.EnsureDBDir(cfg.DatabaseURL)

	// Connect to database (SQLite or PostgreSQL based on DATABASE_URL prefix)
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Run schema migrations â€” create tables if they don't exist
	if err := db.RunMigrations(); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}

	// Run column migrations â€” add new columns to existing tables safely
	if err := db.RunColumnMigrations(); err != nil {
		slog.Error("column migration failed", "error", err)
		os.Exit(1)
	}

	// Start asynchronous background chat worker pool
	workerPool := queue.NewWorkerPool(5, 200)
	workerPool.Start()

	// Construct repositories and services for background tasks
	clientRepo := repository.NewSQLClientRepository(db)
	billingRepo := repository.NewSQLBillingRepository(db)
	activityRepo := repository.NewSQLActivityRepository(db)
	chunkRepo := repository.NewSQLChunkRepository(db)

	embeddingSvc := services.NewEmbeddingService(cfg.GeminiAPIKey)
	ragSvc := services.NewRAGService(chunkRepo, embeddingSvc)
	clientSvc := services.NewClientService(clientRepo, cfg.EncryptionKey)
	billingSvc := services.NewBillingService(billingRepo)
	portalSvc := services.NewPortalService(
		repository.NewSQLPortalRepository(db),
		cfg.EncryptionKey,
		cfg.GeminiAPIKey,
		cfg.OpenAIAPIKey,
		cfg.GroqAPIKey,
		cfg.FallbackGeminiKey,
	)
	chatSvc := services.NewChatService(
		clientRepo,
		activityRepo,
		ragSvc,
		nil,
		cfg.EncryptionKey,
		cfg.WhatsAppVerifyToken,
		cfg.GeminiAPIKey,
		cfg.OpenAIAPIKey,
		cfg.GroqAPIKey,
		cfg.FallbackGeminiKey,
	)

	// -------------------------------------------------------------------------
	// Background: Overdue Invoice Detection
	// Runs every 6 hours to mark invoices past their due date as 'Overdue'.
	// -------------------------------------------------------------------------
	go func() {
		// Run once immediately on startup to catch any existing overdue invoices.
		if err := billingSvc.MarkOverdueInvoices(context.Background()); err != nil {
			slog.Warn("overdue invoice check failed on startup", "error", err)
		}

		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := billingSvc.MarkOverdueInvoices(context.Background()); err != nil {
				slog.Warn("overdue invoice check failed", "error", err)
			}
		}
	}()

	// -------------------------------------------------------------------------
	// Background: Website Auto-Sync
	// Runs every 1 hour to check clients' scraping interval preferences and auto-sync.
	// -------------------------------------------------------------------------
	go func() {
		// Run once on startup after a small delay
		time.Sleep(10 * time.Second)
		clientSvc.AutoSyncClientWebsites(context.Background(), portalSvc)

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			clientSvc.AutoSyncClientWebsites(context.Background(), portalSvc)
		}
	}()

	// -------------------------------------------------------------------------
	// Background: Chat History Auto-Cleanup
	// Runs every 24 hours. Deletes old AI chat logs (NOT tickets) per client's
	// chat_retention_days setting (7, 15, or 30 days).
	// -------------------------------------------------------------------------
	go func() {
		// Initial run after 30 seconds so migrations are settled
		time.Sleep(30 * time.Second)
		if n, err := chatSvc.AutoCleanupChats(context.Background()); err != nil {
			slog.Warn("chat cleanup failed on startup", "error", err)
		} else if n > 0 {
			slog.Info("chat cleanup: old AI chat messages deleted on startup", "count", n)
		}

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if n, err := chatSvc.AutoCleanupChats(context.Background()); err != nil {
				slog.Warn("chat cleanup failed", "error", err)
			} else if n > 0 {
				slog.Info("chat cleanup: old AI chat messages deleted", "count", n)
			}
		}
	}()

	// Build HTTP router with all routes and middleware
	h := router.New(cfg, db, workerPool)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      h,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine so we can handle OS signals below.
	go func() {
		slog.Info("ALVEX backend ready",
			"port",   cfg.Port,
			"health", "http://localhost:"+cfg.Port+"/health",
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT / SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutdown signal received, gracefully stopping")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Warn("forced shutdown", "error", err)
	}
	workerPool.Stop()
	slog.Info("server stopped cleanly")
}
