package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
	aiservice "github.com/codexylab/alvex-backend/pkg/services/ai"
)

// WSHubInterface defines broadcast capability.
type WSHubInterface interface {
	Broadcast(message []byte)
}

// ChatService handles AI chatbot interactions and activity logging.
type ChatService struct {
	ClientRepo        repository.ClientRepository
	ActivityRepo      repository.ActivityRepository
	RAGService        *RAGService
	Hub               WSHubInterface
	EncryptionKey     string
	WhatsAppVerifyTok string
	PlatformGemini    string
	PlatformOpenAI    string
	PlatformGroq      string
	FallbackGemini    string
}

// NewChatService creates a new ChatService instance with optional RAG integration.
func NewChatService(
	clientRepo repository.ClientRepository,
	activityRepo repository.ActivityRepository,
	ragService *RAGService,
	hub WSHubInterface,
	encKey, waTok, platGemini, platOpenAI, platGroq, fallbackGemini string,
) *ChatService {
	return &ChatService{
		ClientRepo:        clientRepo,
		ActivityRepo:      activityRepo,
		RAGService:        ragService,
		Hub:               hub,
		EncryptionKey:     encKey,
		WhatsAppVerifyTok: waTok,
		PlatformGemini:    platGemini,
		PlatformOpenAI:    platOpenAI,
		PlatformGroq:      platGroq,
		FallbackGemini:    fallbackGemini,
	}
}

// detectHandoffTrigger checks if user message indicates an intent to reach a human support representative.
func detectHandoffTrigger(msg string) (bool, string) {
	lower := strings.ToLower(msg)
	triggers := []string{
		"talk to human", "speak to human", "talk to agent", "speak to agent",
		"real person", "human agent", "customer service agent", "talk to support",
		"speak to representative", "connect me to a person", "call me",
		"live agent", "support agent", "human representative", "human please",
		"human", "manager", "supervisor", "insan", "banda", "agent",
		"Ø§Ù†Ø³Ø§Ù†", "Ø¨Ù†Ø¯Û", "Ø§ÛŒØ¬Ù†Ù¹", "Ù†Ù…Ø§Ø¦Ù†Ø¯Û",
	}

	for _, t := range triggers {
		if strings.Contains(lower, t) {
			return true, t
		}
	}
	return false, ""
}

// RegisterTicket directly registers a customer ticket.
func (s *ChatService) RegisterTicket(ctx context.Context, clientID, message, ticketRef, sessionID string) (string, string, error) {
	c, err := s.ClientRepo.GetByID(ctx, clientID)
	if err != nil {
		return "", "", err
	}

	logID := uuid.New().String()
	if err = s.ActivityRepo.InsertActivityLog(ctx, logID, clientID, c.Name, string(models.ChannelWeb), ticketRef, sessionID, message, "", "Pending", 0, 1, "", ""); err != nil {
		return "", "", err
	}

	if s.Hub != nil {
		event := map[string]interface{}{
			"id":         logID,
			"client":     c.Name,
			"type":       models.ChannelWeb,
			"user":       ticketRef,
			"text":       message,
			"status":     "Pending",
			"latency_ms": 0,
			"time":       time.Now().Format(time.RFC3339),
		}
		if eventJSON, err := json.Marshal(event); err == nil {
			s.Hub.Broadcast(eventJSON)
		}
	}

	return logID, c.Name, nil
}

// GetChatHistory fetches chat or ticket history for a session.
func (s *ChatService) GetChatHistory(ctx context.Context, clientID, historyType, ref string) ([]repository.HistoryItem, error) {
	if historyType == "tickets" {
		return s.ActivityRepo.GetWebTicketHistory(ctx, clientID, ref)
	}
	return s.ActivityRepo.GetWebChatHistory(ctx, clientID, ref)
}

// ProcessMessage coordinates rate limits, AI calls, human handoff, and activity logging.
func (s *ChatService) ProcessMessage(ctx context.Context, clientID, userRef, sessionID, message, imageData, channel string) string {
	c, err := s.ClientRepo.GetByID(ctx, clientID)
	if err != nil || c.Status != "Active" {
		return "Service temporarily unavailable."
	}

	// 1. Human Handoff Intent Detection
	if isHandoff, trigger := detectHandoffTrigger(message); isHandoff {
		handoffReply := "I have notified our support team. A human agent has been assigned to your conversation and will get back to you shortly."
		logID := uuid.New().String()
		logMessage := formatLogMessage(message, imageData)

		if logErr := s.ActivityRepo.InsertActivityLog(ctx, logID, clientID, c.Name, channel, userRef, sessionID, logMessage, handoffReply, string(models.ActivityNeedsHuman), 0, 0, "", imageData); logErr != nil {
			slog.Warn("failed to insert handoff activity log", "client_id", clientID, "error", logErr)
		}

		if s.Hub != nil {
			event := map[string]interface{}{
				"id":             logID,
				"client":         c.Name,
				"type":           channel,
				"user":           userRef,
				"text":           logMessage,
				"status":         string(models.ActivityNeedsHuman),
				"needs_human":    true,
				"handoff_reason": trigger,
				"latency_ms":     0,
				"time":           time.Now().Format(time.RFC3339),
			}
			if eventJSON, err := json.Marshal(event); err == nil {
				s.Hub.Broadcast(eventJSON)
			}
		}

		return handoffReply
	}

	apiKey := resolveProviderAPIKey(c, platformKeys{
		EncryptionKey:  s.EncryptionKey,
		Gemini:         s.PlatformGemini,
		OpenAI:         s.PlatformOpenAI,
		Groq:           s.PlatformGroq,
		FallbackGemini: s.FallbackGemini,
	})

	history := s.buildChatHistory(ctx, clientID, sessionID, c, apiKey)
	effectivePrompt := s.buildSystemPrompt(ctx, c, message, imageData)

	start := time.Now()
	status := models.ActivityResolved
	aiReply := "I'm sorry, I could not process your request at this time."

	aiReply, status = s.generateReply(ctx, c, apiKey, effectivePrompt, history, message, imageData, status, aiReply)

	latencyMs := time.Since(start).Milliseconds()
	logMessage := formatLogMessage(message, imageData)

	logID := uuid.New().String()
	if logErr := s.ActivityRepo.InsertActivityLog(ctx, logID, clientID, c.Name, channel, userRef, sessionID, logMessage, aiReply, string(status), latencyMs, 0, "", imageData); logErr != nil {
		slog.Warn("failed to insert activity log", "client_id", clientID, "error", logErr)
	}

	s.broadcastEvent(logID, c.Name, channel, userRef, logMessage, status, latencyMs)

	return aiReply
}

// buildChatHistory fetches session history and summarizes older turns if the conversation is long.
func (s *ChatService) buildChatHistory(ctx context.Context, clientID, sessionID string, c *models.Client, apiKey string) []aiservice.ChatMessage {
	historyLogs, err := s.ActivityRepo.FetchSessionHistory(ctx, clientID, sessionID, 20)
	if err != nil || len(historyLogs) == 0 {
		return nil
	}

	type pair struct{ userMsg, aiMsg string }
	pairs := make([]pair, 0, len(historyLogs))
	for _, h := range historyLogs {
		pairs = append(pairs, pair{h.Message, h.AIResponse})
	}

	// Reverse chronological to chronological
	rawHistory := make([]aiservice.ChatMessage, 0, len(pairs)*2)
	for i := len(pairs) - 1; i >= 0; i-- {
		rawHistory = append(rawHistory, aiservice.ChatMessage{Role: "user", Content: pairs[i].userMsg})
		if pairs[i].aiMsg != "" {
			rawHistory = append(rawHistory, aiservice.ChatMessage{Role: "assistant", Content: pairs[i].aiMsg})
		}
	}

	// Summarization: if history has more than 12 turns, summarize older turns
	if len(rawHistory) > 12 {
		olderTurns := rawHistory[:len(rawHistory)-6]
		recentTurns := rawHistory[len(rawHistory)-6:]

		summary := s.summarizeHistory(ctx, olderTurns, c, apiKey)
		if summary != "" {
			result := []aiservice.ChatMessage{
				{Role: "user", Content: "[Context Summary of prior conversation: " + summary + "]"},
				{Role: "assistant", Content: "Understood. I will remember this context."},
			}
			return append(result, recentTurns...)
		}
	}

	return rawHistory
}

// summarizeHistory uses the AI provider to summarize long conversation context.
func (s *ChatService) summarizeHistory(ctx context.Context, history []aiservice.ChatMessage, c *models.Client, apiKey string) string {
	var sb strings.Builder
	for _, m := range history {
		sb.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}

	provider, err := aiservice.NewProviderWithFallback(string(c.Provider), apiKey, c.Model, s.FallbackGemini)
	if err != nil {
		return ""
	}

	prompt := "You are a conversation summarizer. Condense the following dialogue history into 2-3 key bullet points preserving customer intent, questions asked, and key details."
	summary, err := provider.Chat(prompt, nil, sb.String())
	if err != nil {
		return ""
	}
	return summary
}

// buildSystemPrompt constructs the full system prompt including RAG semantic search, FAQs, and guardrails.
func (s *ChatService) buildSystemPrompt(ctx context.Context, c *models.Client, userQuery, imageData string) string {
	prompt := CompileSystemPersona(c.SystemPersona, c.Name, c.Domain)

	// 1. RAG Vector Search (preferred) or Full Scraped Content (fallback)
	if s.RAGService != nil && userQuery != "" {
		relevantChunks, err := s.RAGService.RetrieveRelevant(ctx, c.ID, userQuery, 4)
		if err == nil && strings.TrimSpace(relevantChunks) != "" {
			prompt += "\n\n--- RELEVANT KNOWLEDGE BASE SECTIONS (Semantic Search) ---\n" + relevantChunks
		} else if c.ScrapedContent != "" {
			prompt += "\n\n--- KNOWLEDGE BASE (from client website) ---\n" + c.ScrapedContent
		}
	} else if c.ScrapedContent != "" {
		prompt += "\n\n--- KNOWLEDGE BASE (from client website) ---\n" + c.ScrapedContent
	}

	if approvedFAQs, err := s.ActivityRepo.GetApprovedFAQs(ctx, c.ID); err == nil && len(approvedFAQs) > 0 {
		faqLines := make([]string, 0, len(approvedFAQs))
		for _, f := range approvedFAQs {
			faqLines = append(faqLines, fmt.Sprintf("Q: %s\nA: %s", f.Question, f.Answer))
		}
		prompt += "\n\n--- PRE-APPROVED FAQs (High Priority) ---\n" +
			"The following are pre-approved Questions and Answers. You MUST answer according to these pairs when matching questions are asked. Do not contradict them.\n\n" +
			strings.Join(faqLines, "\n\n")
	}

	if c.GuardrailsEnabled {
		fallback := "I'm sorry, I can only answer questions related to our services."
		if c.GuardrailsReply != "" {
			fallback = c.GuardrailsReply
		}
		prompt += fmt.Sprintf(
			"\n\n--- GUARDRAILS ACTIVE ---\n"+
				"Strictly restrict your answers to queries about the client's business, services, products, and related industry topics. "+
				"If a user asks about anything unrelated, you MUST respond EXACTLY with: %s. Do not say anything else.",
			fallback,
		)
	}

	prompt += "\n\n--- STRICT RESPONSE FORMATTING RULES ---\n" +
		"1. Keep your replies concise, short, and to the point.\n" +
		"2. Avoid large text blocks. Limit paragraphs to a maximum of 2 sentences.\n" +
		"3. For lists, multi-step instructions, or multiple options, always use bullet points or numbered lists.\n" +
		"4. When referencing pages, products, or sections of the website, always output them as Markdown links: [Link Text](URL). Do not write raw URLs. The URL MUST come from the KNOWLEDGE BASE above.\n" +
		"5. Choose descriptive and helpful Link Text instead of generic words like [click here](url) or [link](url).\n" +
		"6. MULTI-LANGUAGE SUPPORT: Respond in the exact same language and script (e.g. Urdu, Roman Urdu, Arabic, English, Spanish) that the user used in their message."

	if imageData != "" {
		prompt += "\n\n--- VISUAL SEARCH INSTRUCTIONS ---\n" +
			"The user has uploaded an image of a product they want to search for. Your job is to:\n" +
			"1. Identify the product in the image (its name, type, style, color, etc.).\n" +
			"2. Carefully search the client's crawled KNOWLEDGE BASE above to see if this product exists in their catalog.\n" +
			"3. If a matching or very similar product is found, describe it briefly and provide the direct product URL link.\n" +
			"4. If the product is NOT found in the client's KNOWLEDGE BASE, politely state that you could not find this product.\n" +
			"Keep your tone professional and helpful."
	}

	return prompt
}

// generateReply runs FAQ matching first, then falls back to AI provider.
func (s *ChatService) generateReply(
	ctx context.Context,
	c *models.Client,
	apiKey, prompt string,
	history []aiservice.ChatMessage,
	message, imageData string,
	status models.ActivityStatus,
	defaultReply string,
) (string, models.ActivityStatus) {
	approvedFAQs, _ := s.ActivityRepo.GetApprovedFAQs(ctx, c.ID)
	normalizedMsg := NormalizeMsg(message)
	for _, faq := range approvedFAQs {
		if NormalizeMsg(faq.Question) == normalizedMsg {
			return faq.Answer, models.ActivityResolved
		}
	}

	aiProvider, err := aiservice.NewProviderWithFallback(string(c.Provider), apiKey, c.Model, s.FallbackGemini)
	if err != nil {
		return defaultReply, models.ActivityFailed
	}

	var reply string
	var chatErr error
	if imageData != "" {
		reply, chatErr = aiProvider.ChatWithImage(prompt, history, message, imageData)
	} else {
		reply, chatErr = aiProvider.Chat(prompt, history, message)
	}

	if chatErr != nil {
		return defaultReply, models.ActivityFailed
	}
	return reply, models.ActivityResolved
}

// broadcastEvent sends an activity event to connected WebSocket clients.
func (s *ChatService) broadcastEvent(logID, clientName, channel, userRef, message string, status models.ActivityStatus, latencyMs int64) {
	if s.Hub == nil {
		return
	}
	event := map[string]interface{}{
		"id":         logID,
		"client":     clientName,
		"type":       channel,
		"user":       userRef,
		"text":       message,
		"status":     status,
		"latency_ms": latencyMs,
		"time":       time.Now().Format(time.RFC3339),
	}
	if eventJSON, err := json.Marshal(event); err == nil {
		s.Hub.Broadcast(eventJSON)
	}
}

// formatLogMessage prepends [Image Query] prefix if image data is present.
func formatLogMessage(message, imageData string) string {
	if imageData == "" {
		return message
	}
	if message != "" {
		return "[Image Query] " + message
	}
	return "[Image Query]"
}

type modularPersona struct {
	Mode           string `json:"mode"`
	Role           string `json:"role"`
	Tone           string `json:"tone"`
	Length         string `json:"length"`
	Rules          string `json:"rules"`
	Fallback       string `json:"fallback"`
	FallbackCustom string `json:"fallbackCustom"`
}

// CompileSystemPersona builds the final system persona text from raw or modular JSON config.
func CompileSystemPersona(raw string, clientName string, domain string) string {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") {
		return applyTemplateVars(raw, clientName, domain)
	}

	var mod modularPersona
	if err := json.Unmarshal([]byte(raw), &mod); err != nil || mod.Mode == "advanced" {
		return applyTemplateVars(raw, clientName, domain)
	}

	role := applyTemplateVars(orDefault(mod.Role, "customer support agent"), clientName, domain)
	tone := orDefault(mod.Tone, "Friendly & Warm")
	length := orDefault(mod.Length, "Standard")

	var sb strings.Builder
	sb.WriteString("You are a ")
	sb.WriteString(strings.ToLower(tone))
	sb.WriteString(" ")
	sb.WriteString(role)
	sb.WriteString(" representing ")
	sb.WriteString(clientName)
	sb.WriteString(".\n")
	sb.WriteString("Keep your responses ")
	sb.WriteString(strings.ToLower(length))
	sb.WriteString(".\n")

	if rules := strings.TrimSpace(mod.Rules); rules != "" {
		sb.WriteString("Follow these strict rules:\n")
		sb.WriteString(applyTemplateVars(rules, clientName, domain))
		sb.WriteString("\n")
	}

	if mod.Fallback != "" {
		fallback := mod.Fallback
		if fallback == "Custom" && mod.FallbackCustom != "" {
			fallback = mod.FallbackCustom
		}
		sb.WriteString("If you do not know the answer, follow this fallback behavior: ")
		sb.WriteString(applyTemplateVars(fallback, clientName, domain))
		sb.WriteString("\n")
	}

	return sb.String()
}

// NormalizeMsg trims punctuation and lowercases for FAQ matching.
func NormalizeMsg(s string) string {
	s = strings.ToLower(s)
	for _, ch := range []string{"?", "!", ".", ",", "-", "_"} {
		s = strings.ReplaceAll(s, ch, "")
	}
	return strings.TrimSpace(s)
}

// applyTemplateVars replaces {{client_name}} and {{domain}} placeholders.
func applyTemplateVars(s, clientName, domain string) string {
	s = strings.ReplaceAll(s, "{{client_name}}", clientName)
	s = strings.ReplaceAll(s, "{{domain}}", domain)
	return s
}

// orDefault returns val if non-empty, otherwise returns the fallback.
func orDefault(val, fallback string) string {
	if strings.TrimSpace(val) == "" {
		return fallback
	}
	return val
}

// AutoCleanupChats deletes old AI chat messages per client's retention days setting.
func (s *ChatService) AutoCleanupChats(ctx context.Context) (int64, error) {
	clients, _, err := s.ClientRepo.List(ctx, "", "", 1, 1000)
	if err != nil {
		return 0, err
	}

	totalDeleted := int64(0)
	for _, c := range clients {
		if c.ChatRetentionDays > 0 {
			n, err := s.ActivityRepo.CleanupOldChats(ctx, c.ID, c.ChatRetentionDays)
			if err == nil {
				totalDeleted += n
			}
		}
	}
	return totalDeleted, nil
}

// UpdateMessageReaction updates the reaction for a specific chat message.
func (s *ChatService) UpdateMessageReaction(ctx context.Context, id, reaction string) error {
	return s.ActivityRepo.UpdateReaction(ctx, id, reaction)
}
