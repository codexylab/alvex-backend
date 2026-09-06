package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIProviderUsesConfiguredTemperature(t *testing.T) {
	var receivedTemperature float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Temperature float64 `json:"temperature"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		receivedTemperature = payload.Temperature
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIWithConfig("secret", "gpt-test", GenerationConfig{Temperature: 0.25})
	provider.BaseURL = server.URL
	if _, err := provider.Chat("system", nil, "hello"); err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if receivedTemperature != 0.25 {
		t.Fatalf("expected temperature 0.25, got %v", receivedTemperature)
	}
}

func TestFallbackProviderPreservesGenerationConfig(t *testing.T) {
	provider, err := NewProviderWithFallbackConfig(
		"OpenAI",
		"primary-key",
		"OpenAI (GPT-4o)",
		"fallback-key",
		GenerationConfig{Temperature: 0.15},
	)
	if err != nil {
		t.Fatalf("build provider: %v", err)
	}

	fallbackProvider, ok := provider.(*FallbackProvider)
	if !ok {
		t.Fatalf("expected fallback provider, got %T", provider)
	}
	primary := fallbackProvider.Primary.(*OpenAIProvider)
	fallback := fallbackProvider.Fallback.(*GeminiProvider)
	if primary.Config.Temperature != 0.15 || fallback.Config.Temperature != 0.15 {
		t.Fatalf("temperature was not propagated: primary=%v fallback=%v", primary.Config.Temperature, fallback.Config.Temperature)
	}
	if fallback.Model != "gemini-2.0-flash" {
		t.Fatalf("unexpected fallback model: %s", fallback.Model)
	}
}
