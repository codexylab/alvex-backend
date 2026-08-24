package main

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/codexylab/alvex-backend/internal/services/ai"
	"github.com/codexylab/alvex-backend/pkg/crypto"
	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "c:/Users/User/Desktop/Alvex/alvex-backend/alvex_dev.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var (
		name            string
		provider        string
		model           string
		apiKey          string
		geminiAPIKeyRaw sql.NullString
		groqAPIKeyRaw   sql.NullString
		systemPersona   string
	)

	err = db.QueryRow("SELECT name, provider, model, api_key, gemini_api_key, groq_api_key, system_persona FROM clients WHERE id = 'law-firm'").
		Scan(&name, &provider, &model, &apiKey, &geminiAPIKeyRaw, &groqAPIKeyRaw, &systemPersona)
	if err != nil {
		log.Fatalf("Failed to query DB: %v", err)
	}

	encryptionKey := "alvex-encrypt-key-32chars-padded!"
	var selectedKey string

	if provider == "Gemini" && geminiAPIKeyRaw.Valid && geminiAPIKeyRaw.String != "" {
		selectedKey = crypto.DecryptAPIKey(encryptionKey, geminiAPIKeyRaw.String)
	} else if provider == "Groq" && groqAPIKeyRaw.Valid && groqAPIKeyRaw.String != "" {
		selectedKey = crypto.DecryptAPIKey(encryptionKey, groqAPIKeyRaw.String)
	} else {
		selectedKey = crypto.DecryptAPIKey(encryptionKey, apiKey)
	}

	fmt.Printf("Client: %s\n", name)
	fmt.Printf("Provider in DB: %s\n", provider)
	fmt.Printf("Model in DB: %s\n", model)
	fmt.Printf("Decrypted Key starts with: %q (len: %d)\n", selectedKey[:10], len(selectedKey))

	// Create provider
	prov, err := ai.NewProvider(provider, selectedKey, model)
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}

	effectivePrompt := systemPersona + "\n\n--- GUARDRAILS ACTIVE ---\n" +
		"Strictly restrict your answers to queries about the client's business, services, products, and related industry topics. " +
		"If a user asks about anything unrelated (e.g., general knowledge, personal questions, unrelated code, jokes, or other industries), " +
		"you MUST respond EXACTLY with: I am sorry, I can only answer questions related to our legal services. Do not say anything else."

	fmt.Println("\nTesting Chat request with Guardrails prompt...")
	resp, err := prov.Chat(effectivePrompt, nil, "Tell me a short joke about cats.")
	if err != nil {
		fmt.Printf("ERROR calling AI API: %v\n", err)
	} else {
		fmt.Printf("SUCCESS response: %s\n", resp)
	}
}
