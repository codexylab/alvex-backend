package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/codexylab/alvex-backend/internal/database"
	"github.com/codexylab/alvex-backend/internal/handlers"
	_ "modernc.org/sqlite"
)

type chatPayload struct {
	Message   string `json:"message"`
	UserRef   string `json:"user_ref"`
	SessionID string `json:"session_id"`
}

type chatResponse struct {
	Success bool              `json:"success"`
	Data    map[string]string `json:"data"`
}

func main() {
	// 1. Initialize DB
	rawDB, err := sql.Open("sqlite", "c:/Users/User/Desktop/Alvex/alvex-backend/alvex_dev.db")
	if err != nil {
		log.Fatal(err)
	}
	defer rawDB.Close()
	db := database.NewDB(rawDB, "sqlite")

	// 2. Set up WebhookHandler
	h := &handlers.WebhookHandler{
		DB:                db,
		EncryptionKey:     "alvex-encrypt-key-32chars-padded!",
		PlatformGeminiKey: "",
		PlatformOpenAIKey: "",
		PlatformGroqKey:   "",
	}

	// 3. Setup router
	router := chi.NewRouter()
	router.Post("/webhook/chat/{clientId}", h.ReceiveWebChat)

	// Make sure guardrails is disabled first
	_, err = db.Exec("UPDATE clients SET guardrails_enabled = 0, guardrails_reply = NULL WHERE id = 'law-firm'")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("====================================================")
	fmt.Println("TEST 1: Guardrails DISABLED")
	fmt.Println("====================================================")

	// Test 1a: Approved FAQ
	testQuery(router, "Where is your office located?", "Expected: Physical headquarters location at 100 Legal Way")

	// Test 1b: Off-topic query (Guardrails disabled, so it might answer or joke)
	testQuery(router, "Tell me a short joke about cats.", "Expected: A cat joke")

	// Test 1c: Business query matching scraped content (e.g. consultation)
	testQuery(router, "Who is the contact person or how to contact Lex & Counsel?", "Expected: Contact details from Netlify site / Lex & Counsel info")

	fmt.Println("\n====================================================")
	fmt.Println("TEST 2: Guardrails ENABLED")
	fmt.Println("====================================================")

	// Enable Guardrails
	_, err = db.Exec("UPDATE clients SET guardrails_enabled = 1, guardrails_reply = 'I am sorry, I can only answer questions related to our legal services.' WHERE id = 'law-firm'")
	if err != nil {
		log.Fatal(err)
	}

	// Test 2a: Approved FAQ (should still be matched or answered)
	testQuery(router, "What is your main service?", "Expected: Corporate law advice, contract litigation, etc.")

	// Test 2b: Off-topic query (Guardrails active, should trigger fallback EXACTLY)
	testQuery(router, "Tell me a short joke about cats.", "Expected EXACTLY: I am sorry, I can only answer questions related to our legal services.")

	// Test 2c: General programming question (off-topic, should trigger fallback EXACTLY)
	testQuery(router, "How do I write a binary search in Python?", "Expected EXACTLY: I am sorry, I can only answer questions related to our legal services.")

	// Reset guardrails to disabled after test
	_, _ = db.Exec("UPDATE clients SET guardrails_enabled = 0, guardrails_reply = NULL WHERE id = 'law-firm'")
}

func testQuery(router *chi.Mux, query string, expected string) {
	fmt.Printf("\n[QUERY]: %s\n", query)
	fmt.Printf("[EXPECTED]: %s\n", expected)

	payload := chatPayload{
		Message:   query,
		UserRef:   "TestUser",
		SessionID: "TestSession123",
	}
	bodyBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/webhook/chat/law-firm", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	start := time.Now()
	router.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if rec.Code != http.StatusOK {
		fmt.Printf("[RESPONSE ERROR %d]: %s\n", rec.Code, rec.Body.String())
		return
	}

	var res chatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		fmt.Printf("[PARSE ERROR]: %v (Raw: %s)\n", err, rec.Body.String())
		return
	}

	fmt.Printf("[RESPONSE (took %v)]: %s\n", elapsed.Round(time.Millisecond), res.Data["reply"])
	
	// Add delay to avoid hitting LLM API rate limits in automated tests
	time.Sleep(3 * time.Second)
}
