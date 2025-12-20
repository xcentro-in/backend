package httputil

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/shared/config"
)

// CookieManager handles setting and clearing authentication cookies
type CookieManager struct {
	config           *config.Config
	refreshTokenPath string // Base path for refresh token cookies (/v1/auth or /ops/v1/auth)
	accessTokenName  string // Cookie name for access token (access_token or ops_access_token)
	refreshTokenName string // Cookie name for refresh token (refresh_token or ops_refresh_token)
	csrfTokenName    string // Cookie name for CSRF token (csrf_token or ops_csrf_token)
}

// NewCookieManager creates a new cookie manager for regular API (path: /v1/auth)
func NewCookieManager(cfg *config.Config) *CookieManager {
	return &CookieManager{
		config:           cfg,
		refreshTokenPath: "/v1/auth",
		accessTokenName:  "access_token",
		refreshTokenName: "refresh_token",
		csrfTokenName:    "csrf_token",
	}
}

// NewOpsCookieManager creates a new cookie manager for ops dashboard (path: /ops/v1/auth)
func NewOpsCookieManager(cfg *config.Config) *CookieManager {
	return &CookieManager{
		config:           cfg,
		refreshTokenPath: "/ops/v1/auth",
		accessTokenName:  "ops_access_token",
		refreshTokenName: "ops_refresh_token",
		csrfTokenName:    "ops_csrf_token",
	}
}

// getSameSiteMode converts config string to http.SameSite enum
func (cm *CookieManager) getSameSiteMode() http.SameSite {
	switch cm.config.Cookie.SameSite {
	case "Strict":
		return http.SameSiteStrictMode
	case "Lax":
		return http.SameSiteLaxMode
	case "None":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

// SetAccessTokenCookie sets the access token cookie (available on all paths)
func (cm *CookieManager) SetAccessTokenCookie(c echo.Context, token string) {
	cookie := &http.Cookie{
		Name:     cm.accessTokenName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   cm.config.Cookie.Secure,
		SameSite: cm.getSameSiteMode(),
	}
	if cm.config.Cookie.Domain != "" {
		cookie.Domain = cm.config.Cookie.Domain
	}
	c.SetCookie(cookie)
}

// SetRefreshTokenCookie sets the refresh token cookie (restricted to auth routes)
// Path is determined by which constructor was used (NewCookieManager or NewOpsCookieManager)
func (cm *CookieManager) SetRefreshTokenCookie(c echo.Context, token string) {
	cookie := &http.Cookie{
		Name:     cm.refreshTokenName,
		Value:    token,
		Path:     cm.refreshTokenPath,
		HttpOnly: true,
		Secure:   cm.config.Cookie.Secure,
		SameSite: cm.getSameSiteMode(),
	}
	if cm.config.Cookie.Domain != "" {
		cookie.Domain = cm.config.Cookie.Domain
	}
	c.SetCookie(cookie)
}

// ClearAccessTokenCookie clears the access token cookie
func (cm *CookieManager) ClearAccessTokenCookie(c echo.Context) {
	cookie := &http.Cookie{
		Name:     cm.accessTokenName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   cm.config.Cookie.Secure,
		SameSite: cm.getSameSiteMode(),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
	if cm.config.Cookie.Domain != "" {
		cookie.Domain = cm.config.Cookie.Domain
	}
	c.SetCookie(cookie)
}

// ClearRefreshTokenCookie clears the refresh token cookie
// Path is determined by which constructor was used (NewCookieManager or NewOpsCookieManager)
func (cm *CookieManager) ClearRefreshTokenCookie(c echo.Context) {
	cookie := &http.Cookie{
		Name:     cm.refreshTokenName,
		Value:    "",
		Path:     cm.refreshTokenPath,
		HttpOnly: true,
		Secure:   cm.config.Cookie.Secure,
		SameSite: cm.getSameSiteMode(),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
	if cm.config.Cookie.Domain != "" {
		cookie.Domain = cm.config.Cookie.Domain
	}
	c.SetCookie(cookie)
}

// ClearAllAuthCookies clears all authentication cookies (access, refresh, and CSRF tokens)
func (cm *CookieManager) ClearAllAuthCookies(c echo.Context) {
	cm.ClearAccessTokenCookie(c)
	cm.ClearRefreshTokenCookie(c)
	cm.ClearCSRFTokenCookie(c)
}

// GenerateCSRFToken generates a CSRF token using HMAC(session_id, JWT_SECRET)
// Uses RawURLEncoding (no padding) to avoid issues with '=' being stripped from cookies
func (cm *CookieManager) GenerateCSRFToken(sessionID string) string {
	h := hmac.New(sha256.New, []byte(cm.config.JWT.SecretKey))
	h.Write([]byte(sessionID))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// SetCSRFTokenCookie sets the CSRF token cookie (readable by JavaScript)
// MaxAge matches refresh token expiry (720h / 30 days)
//
// SECURITY: Domain is hardcoded to .xcentro.in to allow access from all xcentro.in subdomains
// (app.xcentro.in, staging-app.xcentro.in, ops.xcentro.in, etc.)
func (cm *CookieManager) SetCSRFTokenCookie(c echo.Context, csrfToken string) {
	maxAge := int(cm.config.JWT.RefreshTokenExpiry.Seconds())
	cookie := &http.Cookie{
		Name:     cm.csrfTokenName,
		Value:    csrfToken,
		Domain:   ".xcentro.in", // Leading dot allows all subdomains
		Path:     "/",
		HttpOnly: false, // Frontend JavaScript needs to read this to send in X-CSRF-Token header
		Secure:   cm.config.Cookie.Secure,
		SameSite: cm.getSameSiteMode(),
		MaxAge:   maxAge,
	}

	c.SetCookie(cookie)
}

// ClearCSRFTokenCookie clears the CSRF token cookie
func (cm *CookieManager) ClearCSRFTokenCookie(c echo.Context) {
	cookie := &http.Cookie{
		Name:     cm.csrfTokenName,
		Value:    "",
		Domain:   ".xcentro.in", // Must match the Domain used when setting the cookie
		Path:     "/",
		HttpOnly: false,
		Secure:   cm.config.Cookie.Secure,
		SameSite: cm.getSameSiteMode(),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}

	c.SetCookie(cookie)
}

// ValidateCSRFToken validates the CSRF token from request header against session ID
func (cm *CookieManager) ValidateCSRFToken(sessionID, providedToken string) bool {
	expectedToken := cm.GenerateCSRFToken(sessionID)
	return hmac.Equal([]byte(expectedToken), []byte(providedToken))
}
