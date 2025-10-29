package types

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDate_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantVal string // Expected date in YYYY-MM-DD format
	}{
		{
			name:    "Valid date-only format",
			input:   `"2025-10-15"`,
			wantErr: false,
			wantVal: "2025-10-15",
		},
		{
			name:    "Valid RFC3339 format",
			input:   `"2025-10-15T10:30:00Z"`,
			wantErr: false,
			wantVal: "2025-10-15",
		},
		{
			name:    "Null value",
			input:   `null`,
			wantErr: false,
			wantVal: "",
		},
		{
			name:    "Empty string",
			input:   `""`,
			wantErr: false,
			wantVal: "",
		},
		{
			name:    "Invalid format",
			input:   `"invalid-date"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Date
			err := json.Unmarshal([]byte(tt.input), &d)

			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if tt.wantVal == "" && !d.Time.IsZero() {
					t.Errorf("Expected zero time, got %v", d.Time)
				} else if tt.wantVal != "" {
					got := d.Time.Format("2006-01-02")
					if got != tt.wantVal {
						t.Errorf("UnmarshalJSON() got = %v, want %v", got, tt.wantVal)
					}
				}
			}
		})
	}
}

func TestDate_MarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		date    Date
		want    string
		wantErr bool
	}{
		{
			name: "Valid date",
			date: Date{Time: time.Date(2025, 10, 15, 0, 0, 0, 0, time.UTC)},
			want: `"2025-10-15"`,
		},
		{
			name: "Zero date",
			date: Date{},
			want: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.date)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if string(got) != tt.want {
				t.Errorf("MarshalJSON() got = %v, want %v", string(got), tt.want)
			}
		})
	}
}
