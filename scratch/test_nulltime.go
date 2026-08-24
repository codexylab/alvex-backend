package main

import (
	"database/sql"
	"fmt"
)

func main() {
	var nt sql.NullTime
	err := nt.Scan("2026-06-04 03:39:02.736386 -0700 PDT")
	if err != nil {
		fmt.Printf("Scan failed: %v\n", err)
	} else {
		fmt.Printf("Scan succeeded: %v\n", nt.Time)
	}
}
