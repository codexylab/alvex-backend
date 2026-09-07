package router

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/codexylab/alvex-backend/pkg/config"
	"github.com/codexylab/alvex-backend/pkg/database"
	"github.com/codexylab/alvex-backend/pkg/handlers"
	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/ratelimit"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/services"
)

// New builds and returns the main chi router with all routes registered.
func New(cfg *config.Config, db *database.DB) http.Handler {
	r := chi.NewRouter()
	serverStart := time.Now() // capture for uptime reporting
	widgetSessionRepo := repository.NewSQLWidgetSessionRepository(db)

	// -----------------------------------------------------------------------
	// Global Middleware Stack
	// -----------------------------------------------------------------------
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Recoverer)
	r.Use(middleware.RequestID) // inject X-Request-Id header + context
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.LimitRequestBody(20 << 20))
	r.Use(middleware.Logger)

	// CORS â€” allow configured frontend origins
	r.Use(cors.Handler(cors.Options{
		AllowOriginFunc: func(request *http.Request, origin string) bool {
			if clientID, ok := widgetClientIDFromPath(request.URL.Path); ok {
				return widgetSessionRepo.IsOriginAllowed(request.Context(), clientID, origin)
			}
			for _, allowedOrigin := range cfg.AllowedOrigins {
				if strings.EqualFold(strings.TrimSpace(allowedOrigin), strings.TrimSpace(origin)) {
					return true
				}
			}
			return false
		},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			middleware.OrganizationHeader,
			middleware.ClientHeader,
		},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// -----------------------------------------------------------------------
	// Handler & Service instances
	// -----------------------------------------------------------------------
	wsHub := handlers.NewWSHub(cfg.AllowedOrigins)

	// Rate limiters
	limiter := ratelimit.New(120, time.Minute)
	widgetBootstrapLimiter := ratelimit.New(20, time.Minute)
	machineAPILimiter := ratelimit.New(600, time.Minute)
	// Repositories
	clientRepo := repository.NewTenantSQLClientRepository(db)
	systemClientRepo := repository.NewSQLClientRepository(db)
	organizationRepo := repository.NewSQLOrganizationRepository(db)
	clientMembershipRepo := repository.NewSQLClientMembershipRepository(db)
	manualProvisioningRepo := repository.NewSQLManualProvisioningRepository(db)
	checkoutSignupRepo := repository.NewSQLCheckoutSignupRepository(db)
	stripeRepo := repository.NewSQLStripeRepository(db)
	portalRepo := repository.NewSQLPortalRepository(db)
	billingRepo := repository.NewTenantSQLBillingRepository(db)
	activityRepo := repository.NewSQLActivityRepository(db)
	analyticsRepo := repository.NewSQLAnalyticsRepository(db)
	userRepo := repository.NewSQLUserRepository(db)
	chunkRepo := repository.NewSQLChunkRepository(db)
	docRepo := repository.NewTenantSQLDocumentRepository(db)
	portalDocRepo := repository.NewSQLDocumentRepository(db)
	leadRepo := repository.NewSQLLeadRepository(db)
	handoffRepo := repository.NewSQLHandoffRepository(db)
	auditRepo := repository.NewSQLAuditRepository(db)
	backgroundJobRepo := repository.NewSQLBackgroundJobRepository(db)
	backgroundJobOperationsRepo := repository.NewSQLBackgroundJobOperationsRepository(db)
	apiKeyRepo := repository.NewSQLAPIKeyRepository(db)
	aiUsageRepo := repository.NewSQLAIUsageRepository(db)
	websiteSyncScheduler := services.NewWebsiteIndexScheduler(backgroundJobRepo)
	whatsAppDeliveryRepo := repository.NewSQLWhatsAppDeliveryRepository(db)
	widgetSessionSvc := services.NewWidgetSessionService(widgetSessionRepo)
	apiKeySvc := services.NewAPIKeyService(apiKeyRepo)
	whatsAppWebhookSvc := services.NewWhatsAppWebhookService(
		backgroundJobRepo,
		whatsAppDeliveryRepo,
		whatsAppDeliveryRepo,
	)
	authenticator := middleware.NewAuthenticator(
		cfg.SupabaseURL,
		cfg.SupabaseAnonKey,
		cfg.Env,
		cfg.DevToken,
		userRepo,
	)

	// Core & RAG Services
	embeddingSvc := services.NewEmbeddingService(cfg.GeminiAPIKey)
	ragSvc := services.NewRAGService(chunkRepo, embeddingSvc)
	docSvc := services.NewDocumentService(docRepo, backgroundJobRepo)
	portalDocSvc := services.NewDocumentService(portalDocRepo, backgroundJobRepo)
	clientSvc := services.NewClientService(clientRepo, cfg.EncryptionKey, cfg.PublicAPIURL).
		WithPreviousEncryptionKeys(cfg.PreviousEncryptionKeys)
	systemClientSvc := services.NewClientService(systemClientRepo, cfg.EncryptionKey, cfg.PublicAPIURL).
		WithPreviousEncryptionKeys(cfg.PreviousEncryptionKeys)
	portalSvc := services.NewPortalService(portalRepo, cfg.EncryptionKey, cfg.GeminiAPIKey, cfg.OpenAIAPIKey, cfg.GroqAPIKey, cfg.FallbackGeminiKey).
		WithPreviousEncryptionKeys(cfg.PreviousEncryptionKeys).
		WithAIUsageRepository(aiUsageRepo)
	billingSvc := services.NewBillingService(billingRepo)
	analyticsSvc := services.NewAnalyticsService(analyticsRepo)
	userSvc := services.NewUserService(userRepo)
	stripeSvc := services.NewStripeService(
		stripeRepo,
		stripeRepo,
		checkoutSignupRepo,
		backgroundJobRepo,
		cfg.PublicAPIURL,
	)
	onboardingSvc := services.NewOnboardingService(
		clientSvc,
		websiteSyncScheduler,
		cfg.FrontendURL,
		cfg.PublicAPIURL,
	)
	manualOnboardingSvc := services.NewManualOnboardingService(
		services.NewSupabaseAdminClient(cfg.SupabaseURL, cfg.SupabaseSecretKey),
		manualProvisioningRepo,
		cfg.FrontendURL,
		cfg.PublicAPIURL,
	)
	checkoutSvc := services.NewSelfServiceCheckoutService(
		services.NewStripeCheckoutClient(cfg.StripeSecretKey),
		checkoutSignupRepo,
		map[models.BillingPlan]string{
			models.BillingBasic:      cfg.StripePriceBasic,
			models.BillingPro:        cfg.StripePricePro,
			models.BillingEnterprise: cfg.StripePriceEnterprise,
		},
		cfg.FrontendURL,
	)
	handoffSvc := services.NewHandoffService(handoffRepo)

	chatSvc := services.NewChatService(
		systemClientRepo,
		activityRepo,
		ragSvc,
		wsHub,
		cfg.EncryptionKey,
		cfg.WhatsAppVerifyToken,
		cfg.GeminiAPIKey,
		cfg.OpenAIAPIKey,
		cfg.GroqAPIKey,
		cfg.FallbackGeminiKey,
	).WithPreviousEncryptionKeys(cfg.PreviousEncryptionKeys).
		WithAIUsageRepository(aiUsageRepo)

	// Handlers
	authH := &handlers.AuthHandler{Service: userSvc, OrganizationRepo: organizationRepo}
	clientH := &handlers.ClientHandler{Service: clientSvc, WebsiteSync: websiteSyncScheduler}
	billingH := &handlers.BillingHandler{Service: billingSvc}
	analyticsH := &handlers.AnalyticsHandler{Service: analyticsSvc}
	portalH := &handlers.ClientPortalHandler{Service: portalSvc, IsSQLite: db.IsSQLite()}
	adminScrapeH := &handlers.ScrapeHandler{ClientSvc: clientSvc, WebsiteSync: websiteSyncScheduler}
	portalScrapeH := &handlers.ScrapeHandler{ClientSvc: systemClientSvc, WebsiteSync: websiteSyncScheduler}
	docH := handlers.NewDocumentHandler(docSvc, portalDocSvc)
	handoffH := handlers.NewHandoffHandler(handoffSvc, wsHub)
	stripeH := handlers.NewStripeHandler(stripeSvc, cfg.StripeWebhookSecret)
	onboardingH := handlers.NewOnboardingHandler(onboardingSvc)
	platformClientH := &handlers.PlatformClientHandler{Service: manualOnboardingSvc}
	checkoutH := &handlers.CheckoutHandler{Service: checkoutSvc}
	widgetH := handlers.NewWidgetHandler(widgetSessionSvc, widgetBootstrapLimiter)
	operationsH := handlers.NewOperationsHandler(backgroundJobOperationsRepo, aiUsageRepo, db)
	apiKeyH := handlers.NewAPIKeyHandler(apiKeySvc)
	apiKeyAuthenticator := middleware.NewAPIKeyAuthenticator(apiKeyRepo)

	webhookH := &handlers.WebhookHandler{
		Service:             chatSvc,
		WhatsAppVerifyToken: cfg.WhatsAppVerifyToken,
		WhatsAppAppSecret:   cfg.WhatsAppAppSecret,
		Limiter:             limiter,
		LeadRepo:            leadRepo,
		WhatsApp:            whatsAppWebhookSvc,
	}

	// Health endpoints separate process liveness from dependency readiness.
	healthH := handlers.NewHealthHandler(db, serverStart)
	r.Get("/health", healthH.Ready)
	r.Get("/health/live", healthH.Live)
	r.Get("/health/ready", healthH.Ready)

	// -----------------------------------------------------------------------
	// Public billing webhook. Signature verification is mandatory in the handler.
	// -----------------------------------------------------------------------
	r.Post("/webhooks/stripe", stripeH.HandleWebhook)

	// -----------------------------------------------------------------------
	// WebSocket â€” requires valid session token
	// -----------------------------------------------------------------------
	r.Group(func(r chi.Router) {
		r.Use(authenticator.Middleware)
		r.Use(middleware.RequireOrganization(organizationRepo))
		r.Use(middleware.RequireOrganizationRoles(
			models.OrganizationOwner,
			models.OrganizationAdmin,
			models.OrganizationAgent,
			models.OrganizationBillingManager,
			models.OrganizationViewer,
		))
		r.Get("/ws/activity", wsHub.ServeWS)
	})

	// -----------------------------------------------------------------------
	// Public WhatsApp webhook. Widget traffic uses origin-bound sessions below.
	// -----------------------------------------------------------------------
	r.Route("/webhook", func(r chi.Router) {
		r.Get("/wa/v2/{clientId}", webhookH.VerifyWhatsApp)
		r.Post("/wa/v2/{clientId}", webhookH.ReceiveWhatsApp)
	})

	// Public widget bootstrap validates the embedding origin before issuing a
	// short-lived token. Every other route requires that token and same origin.
	r.Route("/widget/v1/{clientId}", func(r chi.Router) {
		r.Post("/bootstrap", widgetH.Bootstrap)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWidgetSession(widgetSessionSvc))
			r.Post("/messages", webhookH.ReceiveWebChat)
			r.Get("/history", webhookH.GetWebChatHistory)
			r.Patch("/messages/{id}/reaction", webhookH.SaveMessageReaction)
			r.Post("/leads", webhookH.CaptureLead)
			r.Post("/typing", webhookH.TypingIndicator)
			r.Delete("/session", widgetH.Revoke)
		})
	})

	// -----------------------------------------------------------------------
	// Client Portal API — protected by Supabase identity and client membership.
	// -----------------------------------------------------------------------
	r.Route("/api/v1/client-portal", func(r chi.Router) {
		r.Use(authenticator.Middleware)
		r.Use(middleware.RequireClientMembership(clientMembershipRepo))
		r.Get("/me", portalH.Me)
		r.Put("/config", portalH.UpdateConfig)
		r.Get("/stats", portalH.Stats)
		r.Get("/conversations", portalH.Conversations)
		r.Post("/conversations/{id}/reply", portalH.ReplyToConversation)
		r.Get("/bot", portalH.BotInfo)
		r.Get("/billing", portalH.Billing)
		r.Get("/export", portalH.ExportConversations)
		r.Post("/sync-knowledge", portalScrapeH.ScrapePortal)

		// Documents (Knowledge base uploads)
		r.Post("/documents", docH.UploadPortal)
		r.Post("/documents/text", docH.UploadTextPortal)
		r.Get("/documents", docH.ListPortal)
		r.Delete("/documents/{docId}", docH.DeletePortal)
		r.Post("/documents/{docId}/reindex", docH.ReindexPortal)

		// AI Training (FAQs)
		r.Get("/faqs", portalH.GetFAQs)
		r.Post("/faqs", portalH.CreateFAQ)
		r.Put("/faqs/{id}", portalH.UpdateFAQ)
		r.Delete("/faqs/{id}", portalH.DeleteFAQ)
		r.Post("/faqs/generate", portalH.GenerateFAQs)
	})

	// -----------------------------------------------------------------------
	// Machine integration API. API keys are hashed at rest, scoped, expirable,
	// revocable, and may optionally be restricted to one client.
	r.With(
		apiKeyAuthenticator.Middleware,
		middleware.LimitAPIKeyRequests(machineAPILimiter),
		middleware.RequireAPIKeyScopes(models.APIKeyScopeClientsRead),
		middleware.RequireAPIKeyClientAccess("id"),
	).Get("/api/v1/integrations/clients/{id}", clientH.GetOne)
	r.With(
		apiKeyAuthenticator.Middleware,
		middleware.LimitAPIKeyRequests(machineAPILimiter),
		middleware.RequireAPIKeyScopes(models.APIKeyScopeClientsWrite),
		middleware.RequireAPIKeyClientAccess("id"),
	).Put("/api/v1/integrations/clients/{id}", clientH.Update)

	// API v1 â€” Human administration protected by Supabase session tokens.
	// -----------------------------------------------------------------------
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authenticator.Middleware)

		// Auth
		r.Get("/auth/me", authH.Me)

		// Platform administration is independent of any selected tenant.
		r.With(middleware.RequirePlatformRoles(middleware.PlatformSuperAdmin)).
			Post("/platform/clients", platformClientH.Create)
		r.With(middleware.RequirePlatformRoles(middleware.PlatformSuperAdmin)).
			Get("/platform/jobs", operationsH.ListJobs)
		r.With(middleware.RequirePlatformRoles(middleware.PlatformSuperAdmin)).
			Get("/platform/operations", operationsH.Snapshot)
		r.With(middleware.RequirePlatformRoles(middleware.PlatformSuperAdmin)).
			Post("/platform/jobs/{id}/retry", operationsH.RetryJob)

		// Authenticated users can start payment before they own an organization.
		r.Post("/self-service/checkout", checkoutH.Create)

		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrganization(organizationRepo))
			r.Use(middleware.AuditMutations(auditRepo))

			// Machine credentials are managed only by tenant owners/admins. Raw
			// secrets are returned once by POST and never by list/revoke calls.
			r.Route("/api-keys", func(r chi.Router) {
				r.Use(middleware.RequireOrganizationRoles(models.OrganizationOwner, models.OrganizationAdmin))
				r.Get("/", apiKeyH.List)
				r.Post("/", apiKeyH.Create)
				r.Delete("/{id}", apiKeyH.Revoke)
			})

			// Clients
			r.Route("/clients", func(r chi.Router) {
				readClient := middleware.RequireOrganizationRoles(
					models.OrganizationOwner, models.OrganizationAdmin, models.OrganizationAgent,
					models.OrganizationBillingManager, models.OrganizationViewer,
				)
				manageClient := middleware.RequireOrganizationRoles(models.OrganizationOwner, models.OrganizationAdmin)

				r.With(readClient).Get("/", clientH.List)
				r.With(manageClient).Patch("/status", clientH.SetAllStatuses)
				r.With(manageClient).Post("/", clientH.Create)
				r.With(readClient).Get("/{id}", clientH.GetOne)
				r.With(manageClient).Put("/{id}", clientH.Update)
				r.With(manageClient).Patch("/{id}/status", clientH.ToggleStatus)
				r.With(manageClient).Post("/{id}/scrape", adminScrapeH.ScrapeAdmin)
				r.With(manageClient).Delete("/{id}", clientH.Delete)

				// Client Knowledge Documents
				r.With(manageClient).Post("/{id}/documents", docH.UploadAdmin)
				r.With(manageClient).Post("/{id}/documents/text", docH.UploadTextAdmin)
				r.With(readClient).Get("/{id}/documents", docH.ListAdmin)
				r.With(manageClient).Delete("/{id}/documents/{docId}", docH.DeleteAdmin)
				r.With(manageClient).Post("/{id}/documents/{docId}/reindex", docH.ReindexAdmin)
			})

			// Human Handoff & Escalated Conversations
			manageConversations := middleware.RequireOrganizationRoles(
				models.OrganizationOwner, models.OrganizationAdmin, models.OrganizationAgent,
			)
			r.With(manageConversations).Get("/conversations/needs-attention", handoffH.ListNeedsAttention)
			r.With(manageConversations).Post("/conversations/{id}/reply", handoffH.HumanReply)
			r.With(manageConversations).Post("/conversations/{id}/resolve", handoffH.ResolveConversation)

			// 1-Click Automated Onboarding
			manageOrganization := middleware.RequireOrganizationRoles(models.OrganizationOwner, models.OrganizationAdmin)
			r.With(manageOrganization).Post("/onboarding/start", onboardingH.Start)
			r.With(manageOrganization).Get("/onboarding/{id}/status", onboardingH.Status)

			// Billing
			r.Route("/billing", func(r chi.Router) {
				readBilling := middleware.RequireOrganizationRoles(
					models.OrganizationOwner, models.OrganizationAdmin,
					models.OrganizationBillingManager, models.OrganizationViewer,
				)
				manageBilling := middleware.RequireOrganizationRoles(
					models.OrganizationOwner, models.OrganizationAdmin, models.OrganizationBillingManager,
				)
				r.With(readBilling).Get("/stats", billingH.Stats)
				r.With(readBilling).Get("/plans", billingH.Plans)
				r.With(readBilling).Get("/invoices", billingH.ListInvoices)
				r.With(manageBilling).Post("/invoices", billingH.CreateInvoice)
				r.With(manageBilling).Patch("/invoices/{id}/pay", billingH.MarkPaid)
			})

			// Analytics â€” includes Top Questions, Failed Queries, Feedback, CSV export
			r.Route("/analytics", func(r chi.Router) {
				r.Use(middleware.RequireOrganizationRoles(
					models.OrganizationOwner, models.OrganizationAdmin, models.OrganizationAgent,
					models.OrganizationBillingManager, models.OrganizationViewer,
				))
				r.Get("/overview", analyticsH.Overview)
				r.Get("/trends", analyticsH.Trends)
				r.Get("/activity", analyticsH.Activity)
				r.Get("/top-questions", analyticsH.TopQuestions)
				r.Get("/failed-queries", analyticsH.FailedQueries)
				r.Get("/satisfaction", analyticsH.Satisfaction)
				r.Get("/export", analyticsH.ExportCSV) // ?type=clients|invoices|activity
			})
		})
	})

	return r
}

func widgetClientIDFromPath(path string) (string, bool) {
	const prefix = "/widget/v1/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	remainder := strings.TrimPrefix(path, prefix)
	clientID, _, _ := strings.Cut(remainder, "/")
	clientID = strings.TrimSpace(clientID)
	return clientID, clientID != ""
}
