package types

import (
	"fmt"
	"strings"
	"time"
)

// Date is a custom type that handles date-only JSON strings (YYYY-MM-DD format)
// It unmarshals from "2006-01-02" format and stores as time.Time
type Date struct {
	time.Time
}

// UnmarshalJSON implements json.Unmarshaler interface
// Accepts both date-only ("2006-01-02") and RFC3339 formats
func (d *Date) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)

	if s == "null" || s == "" {
		d.Time = time.Time{}
		return nil
	}

	// Try parsing as date-only format first (YYYY-MM-DD)
	t, err := time.Parse("2006-01-02", s)
	if err == nil {
		d.Time = t
		return nil
	}

	// Try parsing as RFC3339 format (with time)
	t, err = time.Parse(time.RFC3339, s)
	if err == nil {
		d.Time = t
		return nil
	}

	return fmt.Errorf("invalid date format: %s (expected YYYY-MM-DD or RFC3339)", s)
}

// MarshalJSON implements json.Marshaler interface
// Always outputs in date-only format (YYYY-MM-DD)
func (d Date) MarshalJSON() ([]byte, error) {
	if d.Time.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + d.Time.Format("2006-01-02") + `"`), nil
}

// ToTimePtr converts Date to *time.Time for compatibility with existing code
func (d *Date) ToTimePtr() *time.Time {
	if d.Time.IsZero() {
		return nil
	}
	return &d.Time
}
