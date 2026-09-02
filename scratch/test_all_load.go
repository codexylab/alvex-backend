package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/codexylab/alvex-backend/pkg/database"
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
	sqlDB, err := sql.Open("sqlite", "./alvex_dev.db")
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()

	db := database.NewDB(sqlDB, "sqlite")

	id := "law-firm"
	c := &Client{}
	var ownerID             sql.NullString
	var portalToken         sql.NullString
	var geminiAPIKeyRaw     sql.NullString
	var groqAPIKeyRaw       sql.NullString
	var groqFallbackEnabled sql.NullBool
	var scrapedContent      sql.NullString
	var scrapeSyncedAt      database.NullTime
	var scrapeEnabled       sql.NullBool
	var scrapeIntervalHours sql.NullInt64

	row := db.QueryRowContext(context.Background(), db.Adapt(`
		SELECT id, name, domain, status, provider, model, api_key,
		       system_persona, webhook_url, temperature, strict_adherence,
		       billing_plan, custom_rate, portal_token, owner_id,
		       COALESCE(gemini_api_key,''), COALESCE(groq_api_key,''),
		       COALESCE(groq_fallback_enabled,0),
		       scraped_content, scrape_synced_at, scrape_enabled, scrape_interval_hours,
		       created_at, updated_at
		FROM clients WHERE id = $1`), id)

	err = row.Scan(
		&c.ID, &c.Name, &c.Domain, &c.Status, &c.Provider, &c.Model,
		&c.APIKey, &c.SystemPersona, &c.WebhookURL, &c.Temperature,
		&c.StrictAdherence, &c.BillingPlan, &c.CustomRate, &portalToken, &ownerID,
		&geminiAPIKeyRaw, &groqAPIKeyRaw, &groqFallbackEnabled,
		&scrapedContent, &scrapeSyncedAt, &scrapeEnabled, &scrapeIntervalHours,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		log.Fatal("Scan failed: ", err)
	}
	
	fmt.Println("🎉 SCAN COMPLETED SUCCESSFULLY!")
	fmt.Printf("Client ID: %s\n", c.ID)
	fmt.Printf("Client Name: %s\n", c.Name)
	if scrapeSyncedAt.Valid {
		fmt.Printf("Scrape Synced At: %v\n", scrapeSyncedAt.Time)
	} else {
		fmt.Println("Scrape Synced At: NULL")
	}
}
