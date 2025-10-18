package httputil

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/shared/config"
)

// CookieManager handles setting and clearing authentication cookies
type CookieManager struct {
	config *config.Config
}

// NewCookieManager creates a new cookie manager with the given configuration
func NewCookieManager(cfg *config.Config) *CookieManager {
	return &CookieManager{
		config: cfg,
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
		Name:     "access_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   cm.config.Cookie.Secure,
		SameSite: cm.getSameSiteMode(),
		MaxAge:   int(cm.config.JWT.AccessTokenExpiry.Seconds()),
	}
	if cm.config.Cookie.Domain != "" {
		cookie.Domain = cm.config.Cookie.Domain
	}
	c.SetCookie(cookie)
}

// SetRefreshTokenCookie sets the refresh token cookie (restricted to /v1/auth routes)
func (cm *CookieManager) SetRefreshTokenCookie(c echo.Context, token string) {
	cookie := &http.Cookie{
		Name:     "refresh_token",
		Value:    token,
		Path:     "/v1/auth",
		HttpOnly: true,
		Secure:   cm.config.Cookie.Secure,
		SameSite: cm.getSameSiteMode(),
		MaxAge:   int(cm.config.JWT.RefreshTokenExpiry.Seconds()),
	}
	if cm.config.Cookie.Domain != "" {
		cookie.Domain = cm.config.Cookie.Domain
	}
	c.SetCookie(cookie)
}

// ClearAccessTokenCookie clears the access token cookie
func (cm *CookieManager) ClearAccessTokenCookie(c echo.Context) {
	cookie := &http.Cookie{
		Name:     "access_token",
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
func (cm *CookieManager) ClearRefreshTokenCookie(c echo.Context) {
	cookie := &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/v1/auth",
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

// ClearAllAuthCookies clears both access and refresh token cookies
func (cm *CookieManager) ClearAllAuthCookies(c echo.Context) {
	cm.ClearAccessTokenCookie(c)
	cm.ClearRefreshTokenCookie(c)
}
