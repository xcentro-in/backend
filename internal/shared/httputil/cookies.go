package httputil

import (
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
}

// NewCookieManager creates a new cookie manager for regular API (path: /v1/auth)
func NewCookieManager(cfg *config.Config) *CookieManager {
	return &CookieManager{
		config:           cfg,
		refreshTokenPath: "/v1/auth",
		accessTokenName:  "access_token",
		refreshTokenName: "refresh_token",
	}
}

// NewOpsCookieManager creates a new cookie manager for ops dashboard (path: /ops/v1/auth)
func NewOpsCookieManager(cfg *config.Config) *CookieManager {
	return &CookieManager{
		config:           cfg,
		refreshTokenPath: "/ops/v1/auth",
		accessTokenName:  "ops_access_token",
		refreshTokenName: "ops_refresh_token",
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

// ClearAllAuthCookies clears both access and refresh token cookies
func (cm *CookieManager) ClearAllAuthCookies(c echo.Context) {
	cm.ClearAccessTokenCookie(c)
	cm.ClearRefreshTokenCookie(c)
}
