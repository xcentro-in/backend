package timeutil

import "time"

// NowUTC returns the current time in UTC.
// Use this instead of time.Now() when storing timestamps to ensure consistent timezone handling.
func NowUTC() time.Time {
	return time.Now().UTC()
}

// ExpiryFromNow returns the expiry timestamp as current UTC time + d.
// Use this to compute expiry times from a configured duration.
func ExpiryFromNow(d time.Duration) time.Time {
	return time.Now().UTC().Add(d)
}

// IsExpired returns true if t is in the past relative to current time.
// Safe to call with a zero time.Time — a zero value is always considered expired.
func IsExpired(t time.Time) bool {
	if t.IsZero() {
		return true
	}
	return time.Now().After(t)
}
