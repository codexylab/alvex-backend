package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ChatMessage represents a single turn in a conversation history.
type ChatMessage struct {
	Role    string // "user" or "assistant"
	Content string
}

// Provider defines the interface all AI providers must implement.
type Provider interface {
	// Chat sends a message with optional conversation history and returns the AI reply.
	Chat(systemPrompt string, history []ChatMessage, userMessage string) (string, error)
	ChatWithImage(systemPrompt string, history []ChatMessage, userMessage string, imageData string) (string, error)
	Name() string
}

// ---- Gemini Provider ----

// GeminiProvider calls the Google Gemini REST API.
type GeminiProvider struct {
	APIKey string
	Model  string
	client *http.Client
}

func NewGemini(apiKey, model string) *GeminiProvider {
	return &GeminiProvider{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *GeminiProvider) Name() string { return "Gemini" }

func (g *GeminiProvider) Chat(systemPrompt string, history []ChatMessage, userMessage string) (string, error) {
	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		g.Model, g.APIKey,
	)

	// Build contents array with full conversation history.
	var contents []map[string]interface{}
	for _, msg := range history {
		role := msg.Role
		if role == "assistant" {
			role = "model" // Gemini uses "model" instead of "assistant"
		}
		contents = append(contents, map[string]interface{}{
			"role":  role,
			"parts": []map[string]string{{"text": msg.Content}},
		})
	}
	contents = append(contents, map[string]interface{}{
		"role":  "user",
		"parts": []map[string]string{{"text": userMessage}},
	})

	payload := map[string]interface{}{
		"system_instruction": map[string]interface{}{
			"parts": []map[string]string{{"text": systemPrompt}},
		},
		"contents": contents,
	}

	body, _ := json.Marshal(payload)
	resp, err := g.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini API error %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("gemini response parse error: %w", err)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned empty response")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}

func (g *GeminiProvider) ChatWithImage(systemPrompt string, history []ChatMessage, userMessage string, imageData string) (string, error) {
	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		g.Model, g.APIKey,
	)

	// Parse base64 imageData
	var mimeType string = "image/jpeg"
	var base64Data string = imageData
	if strings.Contains(imageData, ",") {
		parts := strings.SplitN(imageData, ",", 2)
		if len(parts) == 2 {
			header := parts[0]
			base64Data = parts[1]
			if strings.HasPrefix(header, "data:") {
				subparts := strings.Split(strings.TrimPrefix(header, "data:"), ";")
				if len(subparts) > 0 {
					mimeType = subparts[0]
				}
			}
		}
	}

	// Build contents array with full conversation history.
	var contents []map[string]interface{}
	for _, msg := range history {
		role := msg.Role
		if role == "assistant" {
			role = "model" // Gemini uses "model" instead of "assistant"
		}
		contents = append(contents, map[string]interface{}{
			"role":  role,
			"parts": []map[string]interface{}{{"text": msg.Content}},
		})
	}

	// For the user's message, we send the image and text parts
	userParts := []map[string]interface{}{
		{
			"inline_data": map[string]string{
				"mime_type": mimeType,
				"data":      base64Data,
			},
		},
	}
	if userMessage != "" {
		userParts = append(userParts, map[string]interface{}{"text": userMessage})
	} else {
		userParts = append(userParts, map[string]interface{}{"text": "Identify and find this product."})
	}

	contents = append(contents, map[string]interface{}{
		"role":  "user",
		"parts": userParts,
	})

	payload := map[string]interface{}{
		"system_instruction": map[string]interface{}{
			"parts": []map[string]interface{}{{"text": systemPrompt}},
		},
		"contents": contents,
	}

	body, _ := json.Marshal(payload)
	resp, err := g.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini API error %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("gemini response parse error: %w", err)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned empty response")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}

// ---- OpenAI Provider ----

// OpenAIProvider calls the OpenAI Chat Completions API.
// Also compatible with Groq (same API shape, different base URL).
type OpenAIProvider struct {
	APIKey  string
	Model   string
	BaseURL string
	client  *http.Client
}

func NewOpenAI(apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://api.openai.com/v1",
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func NewGroq(apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://api.groq.com/openai/v1",
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *OpenAIProvider) Name() string { return "OpenAI-compatible" }

func (o *OpenAIProvider) Chat(systemPrompt string, history []ChatMessage, userMessage string) (string, error) {
	// Build messages array with system prompt + history + new message.
	messages := []map[string]string{
		{"role": "system", "content": systemPrompt},
	}
	for _, msg := range history {
		messages = append(messages, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}
	messages = append(messages, map[string]string{"role": "user", "content": userMessage})

	payload := map[string]interface{}{
		"model":       o.Model,
		"messages":    messages,
		"temperature": 0.7,
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", o.BaseURL+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai API error %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("openai response parse error: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai returned empty choices")
	}
	return result.Choices[0].Message.Content, nil
}

func (o *OpenAIProvider) ChatWithImage(systemPrompt string, history []ChatMessage, userMessage string, imageData string) (string, error) {
	// Format the imageData as a data URL if it isn't one
	dataURL := imageData
	if !strings.HasPrefix(imageData, "data:") {
		dataURL = "data:image/jpeg;base64," + imageData
	}

	// Build messages
	var messages []map[string]interface{}
	messages = append(messages, map[string]interface{}{
		"role":    "system",
		"content": systemPrompt,
	})

	for _, msg := range history {
		messages = append(messages, map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	userContent := []map[string]interface{}{
		{
			"type": "image_url",
			"image_url": map[string]interface{}{
				"url": dataURL,
			},
		},
	}
	if userMessage != "" {
		userContent = append(userContent, map[string]interface{}{
			"type": "text",
			"text": userMessage,
		})
	} else {
		userContent = append(userContent, map[string]interface{}{
			"type": "text",
			"text": "Identify and find this product.",
		})
	}

	messages = append(messages, map[string]interface{}{
		"role":    "user",
		"content": userContent,
	})

	payload := map[string]interface{}{
		"model":       o.Model,
		"messages":    messages,
		"temperature": 0.7,
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", o.BaseURL+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai API error %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("openai response parse error: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai returned empty choices")
	}
	return result.Choices[0].Message.Content, nil
}

// ---- FallbackProvider ----

// FallbackProvider wraps a primary provider and automatically retries with a
// secondary provider if the primary fails (e.g. rate limit, quota exhausted, API error).
// This enables automatic Groq → Gemini or OpenAI → Gemini failover.
type FallbackProvider struct {
	Primary  Provider
	Fallback Provider
}

// NewFallbackProvider creates a FallbackProvider that tries Primary first,
// and falls back to Fallback on any error.
func NewFallbackProvider(primary, fallback Provider) *FallbackProvider {
	return &FallbackProvider{Primary: primary, Fallback: fallback}
}

func (f *FallbackProvider) Name() string {
	return fmt.Sprintf("%s→%s(fallback)", f.Primary.Name(), f.Fallback.Name())
}

func (f *FallbackProvider) Chat(systemPrompt string, history []ChatMessage, userMessage string) (string, error) {
	reply, err := f.Primary.Chat(systemPrompt, history, userMessage)
	if err != nil {
		slog.Warn("primary AI provider failed, switching to fallback",
			"primary",  f.Primary.Name(),
			"fallback", f.Fallback.Name(),
			"error",    err.Error(),
		)
		return f.Fallback.Chat(systemPrompt, history, userMessage)
	}
	return reply, nil
}

func (f *FallbackProvider) ChatWithImage(systemPrompt string, history []ChatMessage, userMessage string, imageData string) (string, error) {
	reply, err := f.Primary.ChatWithImage(systemPrompt, history, userMessage, imageData)
	if err != nil {
		slog.Warn("primary AI provider ChatWithImage failed, switching to fallback",
			"primary",  f.Primary.Name(),
			"fallback", f.Fallback.Name(),
			"error",    err.Error(),
		)
		return f.Fallback.ChatWithImage(systemPrompt, history, userMessage, imageData)
	}
	return reply, nil
}

// ---- Model Name Mapping ----

// mapModelName converts UI-friendly display names to vendor API model identifiers.
func mapModelName(provider, displayName string) string {
	switch provider {
	case "Gemini":
		switch displayName {
		case "Gemini 2.5 Flash":
			return "gemini-2.5-flash"
		case "Gemini 2.0 Flash":
			return "gemini-2.0-flash"
		case "Gemini Flash":
			return "gemini-1.5-flash"
		case "Gemini 1.5 Ultra":
			return "gemini-1.5-pro"
		default:
			return "gemini-2.0-flash"
		}
	case "OpenAI":
		switch displayName {
		case "OpenAI (GPT-4o)":
			return "gpt-4o"
		case "OpenAI (GPT-4-turbo)":
			return "gpt-4-turbo"
		default:
			return "gpt-3.5-turbo"
		}
	case "Groq":
		switch displayName {
		case "Groq Llama-3 70B":
			return "llama-3.3-70b-versatile"
		case "Groq Llama-3 8B":
			return "llama-3.1-8b-instant"
		default:
			return "llama-3.1-8b-instant"
		}
	}
	return displayName
}

// NewProvider returns the correct AI provider based on name, with model name mapped.
func NewProvider(providerName, apiKey, model string) (Provider, error) {
	mappedModel := mapModelName(providerName, model)
	switch providerName {
	case "Gemini":
		return NewGemini(apiKey, mappedModel), nil
	case "OpenAI":
		return NewOpenAI(apiKey, mappedModel), nil
	case "Groq":
		return NewGroq(apiKey, mappedModel), nil
	default:
		return nil, fmt.Errorf("unsupported AI provider: %q", providerName)
	}
}

// NewProviderWithFallback returns a provider with automatic Gemini fallback.
// If the primary provider (Groq/OpenAI) fails for any reason, it automatically
// retries the same prompt using the platform-level Gemini API key.
// If providerName is already "Gemini", or fallbackGeminiKey is empty, returns
// the primary provider without wrapping.
func NewProviderWithFallback(providerName, apiKey, model, fallbackGeminiKey string) (Provider, error) {
	primary, err := NewProvider(providerName, apiKey, model)
	if err != nil {
		return nil, err
	}
	// No fallback if already on Gemini or no platform fallback key is set.
	if providerName == "Gemini" || fallbackGeminiKey == "" {
		return primary, nil
	}
	fallback := NewGemini(fallbackGeminiKey, "gemini-pro")
	return NewFallbackProvider(primary, fallback), nil
}
