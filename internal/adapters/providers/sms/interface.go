package sms

import "context"

// SMSMessage represents an SMS message to be sent
type SMSMessage struct {
	To           string // Required: recipient phone number (E.164 format, e.g., +919876543210)
	Body         string // Required: SMS message body
	OriginationID string // Optional: sender ID (overrides default)
}

// Validate checks if the SMS message has required fields
func (s *SMSMessage) Validate() error {
	if s.To == "" {
		return ErrRecipientRequired
	}
	if s.Body == "" {
		return ErrBodyRequired
	}
	return nil
}

// SMSService defines the interface for SMS service providers
// This allows dependency injection and easy swapping of providers (AWS, Twilio, etc.)
type SMSService interface {
	// SendSMS sends a single SMS message
	SendSMS(ctx context.Context, msg *SMSMessage) (messageID string, err error)
}
