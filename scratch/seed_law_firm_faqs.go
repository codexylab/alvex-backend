package main

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "c:/Users/User/Desktop/Alvex/alvex-backend/alvex_dev.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Clear existing FAQs for law-firm first
	_, err = db.Exec("DELETE FROM faqs WHERE client_id = 'law-firm'")
	if err != nil {
		log.Fatal(err)
	}

	faqs := []struct {
		Question   string
		Answer     string
		IsApproved int
	}{
		{
			Question:   "What is your main service?",
			Answer:     "Our main service is professional corporate law advice, contract litigation, and tax dispute resolution.",
			IsApproved: 1,
		},
		{
			Question:   "Where is your office located?",
			Answer:     "Our physical headquarters is at 100 Legal Way, Suite 500, New York, NY.",
			IsApproved: 1,
		},
		{
			Question:   "Do you offer criminal defense services?",
			Answer:     "No, we do not handle criminal defense. We specialize strictly in civil, corporate, and tax law.",
			IsApproved: 1,
		},
		{
			Question:   "How much do you charge?",
			Answer:     "Our consultation fee is $250. Hourly rates vary depending on the partner handling your case.",
			IsApproved: 0, // Keep this as draft
		},
	}

	for _, f := range faqs {
		id := uuid.New().String()
		_, err = db.Exec("INSERT INTO faqs (id, client_id, question, answer, is_approved) VALUES (?, ?, ?, ?, ?)",
			id, "law-firm", f.Question, f.Answer, f.IsApproved)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Inserted FAQ: %s (Approved: %d)\n", f.Question, f.IsApproved)
	}

	fmt.Println("FAQ Seeding Completed!")
}
