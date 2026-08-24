package main

import (
	"database/sql"
	"fmt"
	"log"
	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "c:/Users/User/Desktop/Alvex/alvex-backend/alvex_dev.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var systemPersona string
	var guardrailsEnabled int
	var guardrailsReply sql.NullString
	var scrapedContent sql.NullString
	var status string

	err = db.QueryRow("SELECT system_persona, guardrails_enabled, guardrails_reply, scraped_content, status FROM clients WHERE id = 'law-firm'").Scan(&systemPersona, &guardrailsEnabled, &guardrailsReply, &scrapedContent, &status)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("--- LAW FIRM CONFIGURATION ---\n")
	fmt.Printf("Status: %s\n", status)
	fmt.Printf("Guardrails Enabled: %d\n", guardrailsEnabled)
	if guardrailsReply.Valid {
		fmt.Printf("Guardrails Reply: %s\n", guardrailsReply.String)
	} else {
		fmt.Printf("Guardrails Reply: NULL\n")
	}
	fmt.Printf("\nSystem Persona:\n%s\n", systemPersona)

	fmt.Printf("\nScraped Content Length: %d characters\n", len(scrapedContent.String))

	fmt.Println("\n--- LAW FIRM FAQs ---")
	rows, err := db.Query("SELECT id, question, answer, is_approved FROM faqs WHERE client_id = 'law-firm'")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, q, a string
		var isApproved bool
		rows.Scan(&id, &q, &a, &isApproved)
		fmt.Printf("ID: %s | Approved: %v | Q: %s | A: %s\n", id, isApproved, q, a)
	}
}
