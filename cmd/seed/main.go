package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

func main() {
	godotenv.Load()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}

	var driver, dsn string
	if strings.HasPrefix(dbURL, "sqlite://") {
		driver = "sqlite"
		dsn = strings.TrimPrefix(dbURL, "sqlite://")
	} else {
		driver = "postgres"
		dsn = dbURL
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer db.Close()

	fmt.Printf("🌱 Seeding %s database...\n", driver)

	// 1. Seed User
	userID := "dev-user-001"
	var exists bool
	var query string
	if driver == "sqlite" {
		query = `SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)`
	} else {
		query = `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`
	}
	db.QueryRow(query, userID).Scan(&exists)
	if !exists {
		if driver == "sqlite" {
			_, err = db.Exec(`
				INSERT INTO users (id, email, name, role, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?)`,
				userID, "admin@alvex.ai", "Super Administrator", "admin", time.Now(), time.Now(),
			)
		} else {
			_, err = db.Exec(`
				INSERT INTO users (id, email, name, role, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				userID, "admin@alvex.ai", "Super Administrator", "admin", time.Now(), time.Now(),
			)
		}
		if err != nil {
			log.Fatalf("Failed to seed user: %v", err)
		}
		fmt.Println("✅ Seeded user dev-user-001")
	}

	// 2. Seed Clients
	clients := []struct {
		ID              string
		Name            string
		Domain          string
		Status          string
		Provider        string
		Model           string
		APIKey          string
		SystemPersona   string
		WebhookURL      string
		Temperature     float64
		StrictAdherence bool
		BillingPlan     string
	}{
		{
			ID:              "nexus-dynamics",
			Name:            "Nexus Dynamics",
			Domain:          "nexus-dyn.ai",
			Status:          "Active",
			Provider:        "Gemini",
			Model:           "Gemini Pro",
			APIKey:          "ALVX-NEXD-8921x42b",
			SystemPersona:   "You are a senior customer success representative for Nexus Dynamics. Assist clients with platform configuration, deployment logs, and basic API access questions.\n\nTONE: Technical, concise, and helpful.\nRESTRICTIONS: Avoid offering legal advice. ESCALATE custom cluster billing requests to engineering.",
			WebhookURL:      "http://localhost:8080/webhook/wa/v2/nexus-dynamics",
			Temperature:     0.7,
			StrictAdherence: true,
			BillingPlan:     "Enterprise",
		},
		{
			ID:              "stellar-logic",
			Name:            "Stellar Logic",
			Domain:          "stellar-logic.io",
			Status:          "Suspended",
			Provider:        "Groq",
			Model:           "Groq Llama-3 70B",
			APIKey:          "ALVX-STLL-0422x99f",
			SystemPersona:   "You are an automated logistics support bot for Stellar Logic. Assist users in tracking their orders, verifying shipping dates, and logging support tickets.\n\nTONE: Friendly, structured, and informative.\nRESTRICTIONS: Never issue refunds directly. Refer customers to the human support desk for financial queries.",
			WebhookURL:      "http://localhost:8080/webhook/wa/v2/stellar-logic",
			Temperature:     0.5,
			StrictAdherence: false,
			BillingPlan:     "Pro",
		},
		{
			ID:              "aura-ai",
			Name:            "Aura AI Systems",
			Domain:          "aura-systems.tech",
			Status:          "Active",
			Provider:        "Gemini",
			Model:           "Gemini Flash",
			APIKey:          "ALVX-AURA-3310xz11",
			SystemPersona:   "You are a product specialist for Aura AI Systems. Explain AI capabilities, catalog listings, and starter-pack tiers to site visitors.\n\nTONE: Conversational, upbeat, and persuasive.\nRESTRICTIONS: Stick to standard catalog pricing. Do not negotiate custom discounts.",
			WebhookURL:      "http://localhost:8080/webhook/wa/v2/aura-ai",
			Temperature:     0.8,
			StrictAdherence: true,
			BillingPlan:     "Basic",
		},
		{
			ID:              "kinetix-labs",
			Name:            "Kinetix Labs",
			Domain:          "kinetix.labs",
			Status:          "Active",
			Provider:        "OpenAI",
			Model:           "OpenAI (GPT-4o)",
			APIKey:          "ALVX-KNTX-5592x7d2",
			SystemPersona:   "You are a developer relations agent for Kinetix Labs. Answer endpoint schema questions, troubleshoot SDK requests, and help with rate-limit questions.\n\nTONE: Developer-centric, highly analytical, and code-literate.\nRESTRICTIONS: Do not write entire applications. Provide only focused code snippets.",
			WebhookURL:      "http://localhost:8080/webhook/wa/v2/kinetix-labs",
			Temperature:     0.4,
			StrictAdherence: true,
			BillingPlan:     "Enterprise",
		},
		{
			ID:              "service-shoes",
			Name:            "Service Shoes",
			Domain:          "serviceshoes.com",
			Status:          "Active",
			Provider:        "Gemini",
			Model:           "Gemini Flash",
			APIKey:          "ALVX-SHOE-7781xz09",
			SystemPersona:   "You are a customer service assistant for Service Shoes. Help visitors with product sizes, returns, and order queries.\n\nTONE: Friendly, polite, and helpful.\nRESTRICTIONS: Focus strictly on shoe catalogs and order processes.",
			WebhookURL:      "http://localhost:8080/webhook/wa/v2/service-shoes",
			Temperature:     0.7,
			StrictAdherence: true,
			BillingPlan:     "Basic",
		},
	}

	for _, c := range clients {
		var clientExists bool
		if driver == "sqlite" {
			query = `SELECT EXISTS(SELECT 1 FROM clients WHERE id = ?)`
		} else {
			query = `SELECT EXISTS(SELECT 1 FROM clients WHERE id = $1)`
		}
		db.QueryRow(query, c.ID).Scan(&clientExists)
		if !clientExists {
			if driver == "sqlite" {
				var strictVal int = 0
				if c.StrictAdherence {
					strictVal = 1
				}
				_, err = db.Exec(`
					INSERT INTO clients
					  (id, name, domain, status, provider, model, api_key, system_persona,
					   webhook_url, temperature, strict_adherence, billing_plan, owner_id, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					c.ID, c.Name, c.Domain, c.Status, c.Provider, c.Model, c.APIKey, c.SystemPersona,
					c.WebhookURL, c.Temperature, strictVal, c.BillingPlan, userID, time.Now(), time.Now(),
				)
			} else {
				_, err = db.Exec(`
					INSERT INTO clients
					  (id, name, domain, status, provider, model, api_key, system_persona,
					   webhook_url, temperature, strict_adherence, billing_plan, owner_id, created_at, updated_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
					c.ID, c.Name, c.Domain, c.Status, c.Provider, c.Model, c.APIKey, c.SystemPersona,
					c.WebhookURL, c.Temperature, c.StrictAdherence, c.BillingPlan, userID, time.Now(), time.Now(),
				)
			}
			if err != nil {
				log.Fatalf("Failed to seed client %s: %v", c.ID, err)
			}
			fmt.Printf("   Seeded client: %s\n", c.Name)
		}
	}

	// 3. Seed Invoices
	invoices := []struct {
		ID         string
		ClientID   string
		ClientName string
		Amount     float64
		Status     string
		DueDate    string
	}{
		{ID: "INV-2024-001", ClientID: "nexus-dynamics", ClientName: "Nexus Dynamics", Amount: 1240.00, Status: "Paid", DueDate: "2026-06-20"},
		{ID: "INV-2024-002", ClientID: "stellar-logic", ClientName: "Stellar Logic", Amount: 3500.00, Status: "Pending", DueDate: "2026-06-18"},
		{ID: "INV-2023-998", ClientID: "aura-ai", ClientName: "Aura AI Systems", Amount: 890.00, Status: "Paid", DueDate: "2026-06-10"},
		{ID: "INV-2023-997", ClientID: "kinetix-labs", ClientName: "Kinetix Labs", Amount: 2100.00, Status: "Overdue", DueDate: "2026-05-28"},
	}

	for _, inv := range invoices {
		var invExists bool
		if driver == "sqlite" {
			query = `SELECT EXISTS(SELECT 1 FROM invoices WHERE id = ?)`
		} else {
			query = `SELECT EXISTS(SELECT 1 FROM invoices WHERE id = $1)`
		}
		db.QueryRow(query, inv.ID).Scan(&invExists)
		if !invExists {
			if driver == "sqlite" {
				_, err = db.Exec(`
					INSERT INTO invoices (id, client_id, client_name, amount, status, due_date, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?)`,
					inv.ID, inv.ClientID, inv.ClientName, inv.Amount, inv.Status, inv.DueDate, time.Now().Add(-24*time.Hour),
				)
			} else {
				_, err = db.Exec(`
					INSERT INTO invoices (id, client_id, client_name, amount, status, due_date, created_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7)`,
					inv.ID, inv.ClientID, inv.ClientName, inv.Amount, inv.Status, inv.DueDate, time.Now().Add(-24*time.Hour),
				)
			}
			if err != nil {
				log.Fatalf("Failed to seed invoice %s: %v", inv.ID, err)
			}
			fmt.Printf("   Seeded invoice: %s\n", inv.ID)
		}
	}

	// 4. Seed Activity Logs
	logs := []struct {
		ClientID   string
		ClientName string
		Channel    string
		UserRef    string
		Message    string
		Status     string
		Age        time.Duration
	}{
		{ClientID: "nexus-dynamics", ClientName: "Nexus Dynamics", Channel: "web", UserRef: "Visitor #8291", Message: "How do I reset my password?", Status: "Resolved", Age: 2 * time.Minute},
		{ClientID: "stellar-logic", ClientName: "Stellar Logic", Channel: "whatsapp", UserRef: "+1 202 555-0192", Message: "Pricing for Enterprise plan?", Status: "Handling...", Age: 5 * time.Minute},
		{ClientID: "aura-ai", ClientName: "Aura AI Systems", Channel: "web", UserRef: "Visitor #8288", Message: "Thank you, that helped.", Status: "Archived", Age: 12 * time.Minute},
		{ClientID: "kinetix-labs", ClientName: "Kinetix Labs", Channel: "web", UserRef: "Developer #441", Message: "Endpoint error 429 schema format?", Status: "Resolved", Age: 22 * time.Minute},
	}

	// Check if activity logs table has any rows
	var logCount int
	db.QueryRow(`SELECT COUNT(*) FROM activity_logs`).Scan(&logCount)
	if logCount == 0 {
		for _, l := range logs {
			id := fmt.Sprintf("log-%d", time.Now().UnixNano()+int64(l.Age))
			if driver == "sqlite" {
				_, err = db.Exec(`
					INSERT INTO activity_logs (id, client_id, client_name, channel, user_ref, message, status, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
					id, l.ClientID, l.ClientName, l.Channel, l.UserRef, l.Message, l.Status, time.Now().Add(-l.Age),
				)
			} else {
				_, err = db.Exec(`
					INSERT INTO activity_logs (client_id, client_name, channel, user_ref, message, status, created_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7)`,
					l.ClientID, l.ClientName, l.Channel, l.UserRef, l.Message, l.Status, time.Now().Add(-l.Age),
				)
			}
			if err != nil {
				log.Fatalf("Failed to seed activity log: %v", err)
			}
		}
		fmt.Println("✅ Seeded initial activity logs")
	}

	fmt.Println("🌱 Seeding finished successfully!")
}
