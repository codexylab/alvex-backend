package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"
	_ "modernc.org/sqlite"
)

type Client struct {
	ID                  string       
	Name                string       
	Domain              string       
	Status              string 
	Provider            string   
	Model               string       
	APIKey              string       
	GeminiAPIKey        string 
	GroqAPIKey          string   
	GroqFallbackEnabled bool         
	PortalToken         string 
	SystemPersona       string
	WebhookURL          string
	Temperature         float64
	StrictAdherence     bool
	BillingPlan         string
	CustomRate          *float64
	OwnerID             *string
	ScrapedContent      string
	ScrapeSyncedAt      *time.Time
	ScrapeEnabled       bool
	ScrapeIntervalHours int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func main() {
	db, err := sql.Open("sqlite", "./alvex_dev.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	id := "law-firm"
	c := &Client{}
	var ownerID             sql.NullString
	var portalToken         sql.NullString
	var geminiAPIKeyRaw     sql.NullString
	var groqAPIKeyRaw       sql.NullString
	var groqFallbackEnabled sql.NullBool
	var scrapedContent      sql.NullString
	var scrapeSyncedAt      sql.NullTime
	var scrapeEnabled       sql.NullBool
	var scrapeIntervalHours sql.NullInt64

	err = db.QueryRow(`
		SELECT id, name, domain, status, provider, model, api_key,
		       system_persona, webhook_url, temperature, strict_adherence,
		       billing_plan, custom_rate, portal_token, owner_id,
		       COALESCE(gemini_api_key,''), COALESCE(groq_api_key,''),
		       COALESCE(groq_fallback_enabled,0),
		       scraped_content, scrape_synced_at, scrape_enabled, scrape_interval_hours,
		       created_at, updated_at
		FROM clients WHERE id = ?`, id,
	).Scan(
		&c.ID, &c.Name, &c.Domain, &c.Status, &c.Provider, &c.Model,
		&c.APIKey, &c.SystemPersona, &c.WebhookURL, &c.Temperature,
		&c.StrictAdherence, &c.BillingPlan, &c.CustomRate, &portalToken, &ownerID,
		&geminiAPIKeyRaw, &groqAPIKeyRaw, &groqFallbackEnabled,
		&scrapedContent, &scrapeSyncedAt, &scrapeEnabled, &scrapeIntervalHours,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Client fetched successfully!")
	fmt.Printf("Name: %s\n", c.Name)
}
