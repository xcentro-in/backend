package sms

import "errors"

var (
	// ErrRecipientRequired is returned when recipient phone number is missing
	ErrRecipientRequired = errors.New("recipient phone number is required")

	// ErrBodyRequired is returned when SMS body is missing
	ErrBodyRequired = errors.New("SMS body is required")

	// ErrInvalidPhoneNumber is returned when phone number format is invalid
	ErrInvalidPhoneNumber = errors.New("invalid phone number format (use E.164 format, e.g., +919876543210)")
)
