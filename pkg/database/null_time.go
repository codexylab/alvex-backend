package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// NullTime is a custom database scanner for nullable time columns that works
// across SQLite and PostgreSQL even when dates are returned as raw strings or timezone-abbreviated strings.
type NullTime struct {
	Time  time.Time
	Valid bool
}

// Scan implements the sql.Scanner interface.
func (nt *NullTime) Scan(value interface{}) error {
	if value == nil {
		nt.Time, nt.Valid = time.Time{}, false
		return nil
	}
	nt.Valid = true

	switch v := value.(type) {
	case time.Time:
		nt.Time = v
		return nil
	case []byte:
		return nt.parse(string(v))
	case string:
		return nt.parse(v)
	}

	// Fallback to standard library NullTime scan logic
	var stdNT sql.NullTime
	if err := stdNT.Scan(value); err == nil {
		nt.Time = stdNT.Time
		nt.Valid = stdNT.Valid
		return nil
	}

	return fmt.Errorf("unsupported Scan, storing driver.Value type %T into database.NullTime", value)
}

func (nt *NullTime) parse(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		nt.Valid = false
		return nil
	}

	// Strip Go monotonic clock reading if present (e.g. " m=+87.721858201")
	if idx := strings.Index(s, " m=+"); idx != -1 {
		s = s[:idx]
	}

	// Try various standard and custom time formats
	layouts := []string{
		"2006-01-02 15:04:05.999999999 -0700 MST",
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02T15:04:05.999999999-07:00",
		"2006-01-02T15:04:05.999999999Z",
	}

	var err error
	var t time.Time
	for _, layout := range layouts {
		t, err = time.Parse(layout, s)
		if err == nil {
			nt.Time = t
			return nil
		}
	}
	return fmt.Errorf("failed to parse time string %q: %w", s, err)
}
