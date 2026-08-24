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

	fmt.Println("--- LAST 6 LAW FIRM ACTIVITY LOGS ---")
	rows, err := db.Query("SELECT id, message, ai_response, status, latency_ms FROM activity_logs WHERE client_id = 'law-firm' ORDER BY created_at DESC LIMIT 6")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, msg, resp, status string
		var latency int
		rows.Scan(&id, &msg, &resp, &status, &latency)
		fmt.Printf("ID: %s | Status: %s | Latency: %dms\nMsg: %q\nResp: %q\n\n", id, status, latency, msg, resp)
	}
}
