package email

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"sync"
	textTemplate "text/template"
)

//go:embed templates/*
var templatesFS embed.FS

// EmailType represents the type of email template
type EmailType string

const (
	EmailVerification EmailType = "email_verification"
	PasswordReset     EmailType = "password_reset"
	Invitation        EmailType = "invitation"
)

// EmailVerificationData holds data for email verification template
type EmailVerificationData struct {
	VerificationURL string
}

// PasswordResetData holds data for password reset template
type PasswordResetData struct {
	ResetURL string
}

// InvitationData holds data for invitation template
type InvitationData struct {
	Name     string
	ResetURL string
}

// EmailRenderer handles email template rendering
type EmailRenderer struct {
	htmlTemplates *template.Template
	textTemplates *textTemplate.Template
	mu            sync.RWMutex
}

var (
	rendererInstance *EmailRenderer
	rendererOnce     sync.Once
)

// GetRenderer returns a singleton instance of EmailRenderer
func GetRenderer() *EmailRenderer {
	rendererOnce.Do(func() {
		rendererInstance = &EmailRenderer{}
		if err := rendererInstance.loadTemplates(); err != nil {
			panic(fmt.Sprintf("failed to load email templates: %v", err))
		}
	})
	return rendererInstance
}

// loadTemplates loads all email templates from embedded filesystem
func (r *EmailRenderer) loadTemplates() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Load HTML templates
	htmlTemplates, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return fmt.Errorf("failed to parse HTML templates: %w", err)
	}
	r.htmlTemplates = htmlTemplates

	// Load text templates
	textTemplates, err := textTemplate.ParseFS(templatesFS, "templates/*.txt")
	if err != nil {
		return fmt.Errorf("failed to parse text templates: %w", err)
	}
	r.textTemplates = textTemplates

	return nil
}

// RenderHTML renders an HTML email template with the provided data
func (r *EmailRenderer) RenderHTML(emailType EmailType, data interface{}) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	templateName := string(emailType) + ".html"

	var buf bytes.Buffer
	if err := r.htmlTemplates.ExecuteTemplate(&buf, templateName, data); err != nil {
		return "", fmt.Errorf("failed to render HTML template %s: %w", templateName, err)
	}

	return buf.String(), nil
}

// RenderText renders a text email template with the provided data
func (r *EmailRenderer) RenderText(emailType EmailType, data interface{}) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	templateName := string(emailType) + ".txt"

	var buf bytes.Buffer
	if err := r.textTemplates.ExecuteTemplate(&buf, templateName, data); err != nil {
		return "", fmt.Errorf("failed to render text template %s: %w", templateName, err)
	}

	return buf.String(), nil
}

// RenderEmailVerification renders email verification templates
func (r *EmailRenderer) RenderEmailVerification(verificationURL string) (htmlBody, textBody string, err error) {
	data := EmailVerificationData{
		VerificationURL: verificationURL,
	}

	htmlBody, err = r.RenderHTML(EmailVerification, data)
	if err != nil {
		return "", "", err
	}

	textBody, err = r.RenderText(EmailVerification, data)
	if err != nil {
		return "", "", err
	}

	return htmlBody, textBody, nil
}

// RenderPasswordReset renders password reset templates
func (r *EmailRenderer) RenderPasswordReset(resetURL string) (htmlBody, textBody string, err error) {
	data := PasswordResetData{
		ResetURL: resetURL,
	}

	htmlBody, err = r.RenderHTML(PasswordReset, data)
	if err != nil {
		return "", "", err
	}

	textBody, err = r.RenderText(PasswordReset, data)
	if err != nil {
		return "", "", err
	}

	return htmlBody, textBody, nil
}

// RenderInvitation renders invitation templates
func (r *EmailRenderer) RenderInvitation(name, resetURL string) (htmlBody, textBody string, err error) {
	data := InvitationData{
		Name:     name,
		ResetURL: resetURL,
	}

	htmlBody, err = r.RenderHTML(Invitation, data)
	if err != nil {
		return "", "", err
	}

	textBody, err = r.RenderText(Invitation, data)
	if err != nil {
		return "", "", err
	}

	return htmlBody, textBody, nil
}
