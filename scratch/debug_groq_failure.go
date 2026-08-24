package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/codexylab/alvex-backend/internal/database"
	"github.com/codexylab/alvex-backend/internal/services/ai"
	"github.com/codexylab/alvex-backend/pkg/crypto"
	_ "modernc.org/sqlite"
)

func main() {
	rawDB, err := sql.Open("sqlite", "c:/Users/User/Desktop/Alvex/alvex-backend/alvex_dev.db")
	if err != nil {
		log.Fatal(err)
	}
	defer rawDB.Close()
	db := database.NewDB(rawDB, "sqlite")

	// 1. Fetch client config
	var (
		clientName      string
		clientDomain    string
		provider        string
		model           string
		apiKey          string
		geminiAPIKeyRaw sql.NullString
		groqAPIKeyRaw   sql.NullString
		systemPersona   string
		scrapedContent  sql.NullString
	)

	err = db.QueryRow("SELECT name, domain, provider, model, api_key, gemini_api_key, groq_api_key, system_persona, scraped_content FROM clients WHERE id = 'law-firm'").
		Scan(&clientName, &clientDomain, &provider, &model, &apiKey, &geminiAPIKeyRaw, &groqAPIKeyRaw, &systemPersona, &scrapedContent)
	if err != nil {
		log.Fatal(err)
	}

	encryptionKey := "alvex-encrypt-key-32chars-padded!"
	var decryptedKey string
	if provider == "Groq" && groqAPIKeyRaw.Valid && groqAPIKeyRaw.String != "" {
		decryptedKey = crypto.DecryptAPIKey(encryptionKey, groqAPIKeyRaw.String)
	} else {
		decryptedKey = crypto.DecryptAPIKey(encryptionKey, apiKey)
	}

	// 2. Fetch history
	rows, err := db.Query(`
		SELECT message, ai_response
		FROM   activity_logs
		WHERE  client_id  = 'law-firm'
		  AND  session_id = 'TestSession123'
		  AND  status     = 'Resolved'
		ORDER BY created_at DESC
		LIMIT 10`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	type pair struct{ userMsg, aiMsg string }
	var pairs []pair
	for rows.Next() {
		var msg, aiResp string
		rows.Scan(&msg, &aiResp)
		pairs = append(pairs, pair{msg, aiResp})
	}

	var history []ai.ChatMessage
	for i := len(pairs) - 1; i >= 0; i-- {
		history = append(history, ai.ChatMessage{Role: "user", Content: pairs[i].userMsg})
		if pairs[i].aiMsg != "" {
			history = append(history, ai.ChatMessage{Role: "assistant", Content: pairs[i].aiMsg})
		}
	}

	// 3. Compile prompt
	effectivePrompt := systemPersona
	if scrapedContent.Valid && scrapedContent.String != "" {
		effectivePrompt += "\n\n--- KNOWLEDGE BASE (from client website) ---\n" + scrapedContent.String
	}

	// FAQs
	var faqsText []string
	faqRows, _ := db.Query("SELECT question, answer FROM faqs WHERE client_id = 'law-firm' AND is_approved = 1 ORDER BY created_at ASC")
	defer faqRows.Close()
	for faqRows.Next() {
		var q, a string
		faqRows.Scan(&q, &a)
		faqsText = append(faqsText, fmt.Sprintf("Q: %s\nA: %s", q, a))
	}
	if len(faqsText) > 0 {
		effectivePrompt += "\n\n--- PRE-APPROVED FAQs (High Priority) ---\n" +
			"The following are pre-approved Questions and Answers. You MUST answer according to these pairs when matching questions are asked. Do not contradict them.\n\n" +
			strings.Join(faqsText, "\n\n")
	}

	// Guardrails (assume enabled)
	fallback := "I am sorry, I can only answer questions related to our legal services."
	effectivePrompt += fmt.Sprintf("\n\n--- GUARDRAILS ACTIVE ---\n"+
		"Strictly restrict your answers to queries about the client's business, services, products, and related industry topics. "+
		"If a user asks about anything unrelated (e.g., general knowledge, personal questions, unrelated code, jokes, or other industries), "+
		"you MUST respond EXACTLY with: %s. Do not say anything else.", fallback)

	effectivePrompt += "\n\n--- STRICT RESPONSE FORMATTING RULES ---\n" +
		"1. Keep your replies extremely concise, short, and to the point.\n" +
		"2. Avoid large text blocks. Limit paragraphs to a maximum of 2 sentences.\n" +
		"3. For lists, multi-step instructions, or multiple options, always use bullet points or numbered lists.\n" +
		"4. When referencing pages, products, or sections of the website, always output them as Markdown links: [Link Text](URL). Do not write raw URLs. The URL MUST come from the KNOWLEDGE BASE above. Do not hallucinate or make up links.\n" +
		"5. Choose descriptive and helpful Link Text (e.g., [Our Products](url) or [Contact Support](url)) instead of generic words like [click here](url) or [link](url).\n" +
		"6. Match the language, tone, and script of the user's message (e.g., if the user asks in Roman Urdu, reply in Roman Urdu; if Urdu script, reply in Urdu script; if English, reply in English)."

	// 4. Call Groq
	prov, err := ai.NewProvider(provider, decryptedKey, model)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("History len: %d\n", len(history))
	for idx, hMsg := range history {
		fmt.Printf("  [%d] %s: %q\n", idx, hMsg.Role, hMsg.Content)
	}

	fmt.Println("\nCalling Groq Chat...")
	resp, err := prov.Chat(effectivePrompt, history, "Tell me a short joke about cats.")
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("SUCCESS: %q\n", resp)
	}
}
