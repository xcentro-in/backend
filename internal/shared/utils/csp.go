package utils

import "fmt"

// GenerateCSPHeader generates a Content-Security-Policy header string
// If nonce is provided (non-empty), it will be included in script-src and style-src directives
// If nonce is empty, the CSP header will use 'unsafe-inline' fallback (for compatibility)
func GenerateCSPHeader(nonce string) string {
	if nonce != "" {
		// With nonce - strict CSP policy
		return fmt.Sprintf(
			"default-src 'self'; "+
				"script-src 'self' 'nonce-%s'; "+
				"style-src 'self' 'nonce-%s'; "+
				"object-src 'none'; "+
				"frame-ancestors 'none'; "+
				"img-src 'self' data:;",
			nonce, nonce,
		)
	}

	// Without nonce - base CSP policy (for http_server.go and other non-HTML responses)
	return "default-src 'self'; " +
		"script-src 'self' 'unsafe-inline'; " +
		"style-src 'self' 'unsafe-inline'; " +
		"object-src 'none'; " +
		"frame-ancestors 'none'; " +
		"img-src 'self' data:;"
}
