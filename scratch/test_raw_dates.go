package main

import (
	"database/sql"
	"fmt"
	"log"
	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "./alvex_dev.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var createdAt, updatedAt string
	err = db.QueryRow("SELECT created_at, updated_at FROM clients WHERE id = ?", "law-firm").Scan(&createdAt, &updatedAt)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("created_at raw: %q\n", createdAt)
	fmt.Printf("updated_at raw: %q\n", updatedAt)
}
