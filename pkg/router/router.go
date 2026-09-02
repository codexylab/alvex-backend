package router

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/codexylab/alvex-backend/pkg/config"
	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/handlers"
	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/queue"
	"github.com/codexylab/alvex-backend/pkg/ratelimit"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/services"
)

// New builds and returns the main chi router with all routes registered.
func New(cfg *config.Config, db *database.DB, workerPool *queue.WorkerPool) http.Handler {
	r := chi.NewRouter()
	serverStart := time.Now() // capture for uptime reporting

	// -----------------------------------------------------------------------
	// Global Middleware Stack
	// -----------------------------------------------------------------------
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Recoverer)
	r.Use(middleware.RequestID) // inject X-Request-Id header + context
	r.Use(middleware.Logger)

	// CORS â€” allow configured frontend origins
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// -----------------------------------------------------------------------
	// Handler & Service instances
	// -----------------------------------------------------------------------
	wsHub := handlers.NewWSHub()

	// Rate limiters
	limiter := ratelimit.New(120, time.Minute)
	dailyLimiter := ratelimit.NewDailyLimiter()

	// Repositories
	clientRepo := repository.NewSQLClientRepository(db)
	portalRepo := repository.NewSQLPortalRepository(db)
	billingRepo := repository.NewSQLBillingRepository(db)
	activityRepo := repository.NewSQLActivityRepository(db)
	analyticsRepo := repository.NewSQLAnalyticsRepository(db)
	userRepo := repository.NewSQLUserRepository(db)
	chunkRepo := repository.NewSQLChunkRepository(db)
	docRepo := repository.NewSQLDocumentRepository(db)
	leadRepo := repository.NewSQLLeadRepository(db)

	// Core & RAG Services
	embeddingSvc := services.NewEmbeddingService(cfg.GeminiAPIKey)
	ragSvc := services.NewRAGService(chunkRepo, embeddingSvc)
	docSvc := services.NewDocumentService(docRepo, ragSvc)
	clientSvc := services.NewClientService(clientRepo, cfg.EncryptionKey)
	portalSvc := services.NewPortalService(portalRepo, cfg.EncryptionKey, cfg.GeminiAPIKey, cfg.OpenAIAPIKey, cfg.GroqAPIKey, cfg.FallbackGeminiKey)
	billingSvc := services.NewBillingService(billingRepo)
	analyticsSvc := services.NewAnalyticsService(analyticsRepo)
	userSvc := services.NewUserService(userRepo)
	stripeSvc := services.NewStripeService(billingRepo, clientRepo)
	onboardingSvc := services.NewOnboardingService(clientSvc, portalSvc, ragSvc)

	chatSvc := services.NewChatService(
		clientRepo,
		activityRepo,
		ragSvc,
		wsHub,
		cfg.EncryptionKey,
		cfg.WhatsAppVerifyToken,
		cfg.GeminiAPIKey,
		cfg.OpenAIAPIKey,
		cfg.GroqAPIKey,
		cfg.FallbackGeminiKey,
	)

	// Handlers
	authH := &handlers.AuthHandler{Service: userSvc}
	clientH := &handlers.ClientHandler{Service: clientSvc, PortalSvc: portalSvc}
	billingH := &handlers.BillingHandler{Service: billingSvc}
	analyticsH := &handlers.AnalyticsHandler{Service: analyticsSvc}
	portalH := &handlers.ClientPortalHandler{Service: portalSvc, IsSQLite: db.IsSQLite()}
	scrapeH := &handlers.ScrapeHandler{ClientSvc: clientSvc, PortalSvc: portalSvc}
	docH := handlers.NewDocumentHandler(docSvc)
	handoffH := handlers.NewHandoffHandler(db, wsHub)
	stripeH := handlers.NewStripeHandler(stripeSvc, cfg.StripeWebhookSecret)
	onboardingH := handlers.NewOnboardingHandler(onboardingSvc)

	webhookH := &handlers.WebhookHandler{
		Service:             chatSvc,
		WhatsAppVerifyToken: cfg.WhatsAppVerifyToken,
		WhatsAppAppSecret:   cfg.WhatsAppAppSecret,
		Limiter:             limiter,
		DailyLimiter:        dailyLimiter,
		WorkerPool:          workerPool,
		LeadRepo:            leadRepo,
	}

	// -----------------------------------------------------------------------
	// Health check (public) â€” includes DB ping, uptime, and WS client count
	// -----------------------------------------------------------------------
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		dbStatus := "ok"
		if err := db.Ping(); err != nil {
			dbStatus = "error: " + err.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":     "ok",
			"service":    "alvex-backend",
			"uptime":     time.Since(serverStart).Round(time.Second).String(),
			"db":         dbStatus,
			"ws_clients": wsHub.ConnectedCount(),
		})
	})

	// -----------------------------------------------------------------------
	// Public Webhooks â€” Clerk, Stripe, WhatsApp, and Web Chat
	// -----------------------------------------------------------------------
	r.Post("/webhooks/clerk", authH.ClerkWebhook)
	r.Post("/webhooks/stripe", stripeH.HandleWebhook)

	// -----------------------------------------------------------------------
	// WebSocket â€” requires valid session token
	// -----------------------------------------------------------------------
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate)
		r.Get("/ws/activity", wsHub.ServeWS)
	})

	// -----------------------------------------------------------------------
	// Public Webhooks â€” WhatsApp & Web Widget
	// -----------------------------------------------------------------------
	r.Route("/webhook", func(r chi.Router) {
		r.Get("/wa/v2/{clientId}", webhookH.VerifyWhatsApp)
		r.Post("/wa/v2/{clientId}", webhookH.ReceiveWhatsApp)
		r.Post("/chat/{clientId}", webhookH.ReceiveWebChat)
		r.Get("/chat/{clientId}/history", webhookH.GetWebChatHistory)
		r.Patch("/chat/message/{id}/reaction", webhookH.SaveMessageReaction)
		r.Post("/chat/{clientId}/lead", webhookH.CaptureLead)
		r.Post("/chat/{clientId}/typing", webhookH.TypingIndicator)
	})

	// -----------------------------------------------------------------------
	// Client Portal API â€” Protected by client portal token
	// -----------------------------------------------------------------------
	r.Route("/api/v1/client-portal", func(r chi.Router) {
		r.Use(middleware.AuthenticateClientPortal(db))
		r.Get("/me", portalH.Me)
		r.Put("/config", portalH.UpdateConfig)
		r.Get("/stats", portalH.Stats)
		r.Get("/conversations", portalH.Conversations)
		r.Post("/conversations/{id}/reply", portalH.ReplyToConversation)
		r.Get("/bot", portalH.BotInfo)
		r.Get("/billing", portalH.Billing)
		r.Get("/export", portalH.ExportConversations)
		r.Post("/sync-knowledge", scrapeH.ScrapePortal)

		// Documents (Knowledge base uploads)
		r.Post("/documents", docH.UploadPortal)
		r.Get("/documents", docH.ListPortal)
		r.Delete("/documents/{docId}", docH.DeletePortal)

		// AI Training (FAQs)
		r.Get("/faqs", portalH.GetFAQs)
		r.Post("/faqs", portalH.CreateFAQ)
		r.Put("/faqs/{id}", portalH.UpdateFAQ)
		r.Delete("/faqs/{id}", portalH.DeleteFAQ)
		r.Post("/faqs/generate", portalH.GenerateFAQs)
	})

	// -----------------------------------------------------------------------
	// API v1 â€” All protected by session token
	// -----------------------------------------------------------------------
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.Authenticate)

		// Auth
		r.Get("/auth/me", authH.Me)

		// Clients
		r.Route("/clients", func(r chi.Router) {
			r.Get("/", clientH.List)
			r.Post("/", clientH.Create)
			r.Get("/{id}", clientH.GetOne)
			r.Put("/{id}", clientH.Update)
			r.Patch("/{id}/status", clientH.ToggleStatus)
			r.Post("/{id}/rotate-token", clientH.RotateToken)
			r.Post("/{id}/rotate-key", clientH.RotateKey)
			r.Post("/{id}/scrape", scrapeH.ScrapeAdmin)
			r.Delete("/{id}", clientH.Delete)

			// Client Knowledge Documents
			r.Post("/{id}/documents", docH.UploadAdmin)
			r.Get("/{id}/documents", docH.ListAdmin)
			r.Delete("/{id}/documents/{docId}", docH.DeleteAdmin)
		})

		// Human Handoff & Escalated Conversations
		r.Get("/conversations/needs-attention", handoffH.ListNeedsAttention)
		r.Post("/conversations/{id}/reply", handoffH.HumanReply)
		r.Post("/conversations/{id}/resolve", handoffH.ResolveConversation)

		// 1-Click Automated Onboarding
		r.Post("/onboarding/start", onboardingH.Start)
		r.Get("/onboarding/{id}/status", onboardingH.Status)

		// Billing
		r.Route("/billing", func(r chi.Router) {
			r.Get("/stats", billingH.Stats)
			r.Get("/invoices", billingH.ListInvoices)
			r.Post("/invoices", billingH.CreateInvoice)
			r.Patch("/invoices/{id}/pay", billingH.MarkPaid)
		})

		// Analytics â€” includes Top Questions, Failed Queries, Feedback, CSV export
		r.Route("/analytics", func(r chi.Router) {
			r.Get("/overview", analyticsH.Overview)
			r.Get("/trends", analyticsH.Trends)
			r.Get("/activity", analyticsH.Activity)
			r.Get("/top-questions", analyticsH.TopQuestions)
			r.Get("/failed-queries", analyticsH.FailedQueries)
			r.Get("/satisfaction", analyticsH.Satisfaction)
			r.Get("/export", analyticsH.ExportCSV) // ?type=clients|invoices|activity
		})
	})

	return r
}

