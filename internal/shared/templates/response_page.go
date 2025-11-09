package templates

import "fmt"

// PageType represents the type/status of the response page
type PageType string

const (
	PageTypeSuccess PageType = "success"
	PageTypeError   PageType = "error"
	PageTypeInfo    PageType = "info"
	PageTypeWarning PageType = "warning"
	PageTypePending PageType = "pending"
)

// ResponsePageConfig contains configuration for generating response HTML pages
type ResponsePageConfig struct {
	PageType       PageType // Type of page (success, error, info, warning, pending)
	Title          string   // Main title/heading
	Message        string   // Primary message
	SubMessage     string   // Optional secondary message
	AutoCloseSec   int      // Auto-close window after N seconds (0 = no auto-close)
	ShowCloseHint  bool     // Show manual close hint
	AdditionalHTML string   // Optional additional HTML content
}

// GenerateResponsePage generates a fully styled HTML response page
func GenerateResponsePage(config ResponsePageConfig) string {
	// Set defaults
	if config.AutoCloseSec == 0 {
		config.AutoCloseSec = 2 // Default 2 seconds
	}

	// Get theme based on page type
	theme := getTheme(config.PageType)

	// Build auto-close script
	autoCloseScript := ""
	if config.AutoCloseSec > 0 {
		autoCloseScript = fmt.Sprintf(`
        (function() {
            // Close window after %d seconds
            setTimeout(function() {
                window.close();
            }, %d);
        })();`, config.AutoCloseSec, config.AutoCloseSec*1000)
	}

	// Build close hint
	closeHint := ""
	if config.ShowCloseHint {
		closeHintText := "This window will close automatically in 2 seconds..."
		if config.AutoCloseSec != 2 {
			closeHintText = fmt.Sprintf("This window will close automatically in %d seconds...", config.AutoCloseSec)
		}
		closeHint = fmt.Sprintf(`
        <p>%s</p>
        <p style="font-size: 14px; color: #80868b;">If not closed automatically, please close this window manually.</p>`, closeHintText)
	}

	// Build submessage
	subMessageHTML := ""
	if config.SubMessage != "" {
		subMessageHTML = fmt.Sprintf(`
        <div class="message">
            <p>%s</p>
        </div>`, config.SubMessage)
	}

	// Build additional HTML
	additionalHTML := ""
	if config.AdditionalHTML != "" {
		additionalHTML = config.AdditionalHTML
	}

	return fmt.Sprintf(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s - xcentro</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: %s;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 16px;
            padding: 48px 32px;
            max-width: 480px;
            width: 100%%;
            text-align: center;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.15);
        }
        .icon {
            width: 80px;
            height: 80px;
            margin: 0 auto 24px;
            background: %s;
            border-radius: 50%%;
            display: flex;
            align-items: center;
            justify-content: center;
            animation: scaleIn 0.5s ease-out;
        }
        .icon svg {
            width: 48px;
            height: 48px;
            stroke: white;
            stroke-width: 3;
            fill: none;
        }
        h1 {
            color: #202124;
            font-size: 28px;
            font-weight: 600;
            margin-bottom: 12px;
        }
        p {
            color: #5f6368;
            font-size: 16px;
            line-height: 1.6;
            margin-bottom: 24px;
        }
        .message {
            background: %s;
            border-left: 4px solid %s;
            padding: 16px;
            border-radius: 8px;
            margin-bottom: 24px;
            text-align: left;
        }
        .message p {
            color: %s;
            margin: 0;
            font-size: 14px;
        }
        .footer {
            color: #80868b;
            font-size: 14px;
            margin-top: 32px;
        }
        @keyframes scaleIn {
            from {
                transform: scale(0);
                opacity: 0;
            }
            to {
                transform: scale(1);
                opacity: 1;
            }
        }
    </style>
    <script>
        %s
    </script>
</head>
<body>
    <div class="container">
        <div class="icon">
            %s
        </div>
        <h1>%s</h1>
        <p>%s</p>
        %s
        %s
        %s
        <div class="footer">
            <p><strong>xcentro</strong><br>
            MerchantBridge Fintech Solutions Pvt Ltd</p>
        </div>
    </div>
</body>
</html>
`,
		config.Title,                // Page title
		theme.backgroundGradient,    // Background gradient
		theme.iconColor,             // Icon background color
		theme.messageBackground,     // Message box background
		theme.iconColor,             // Message box border color
		theme.messageTextColor,      // Message text color
		autoCloseScript,             // Auto-close script
		theme.iconSVG,               // Icon SVG
		config.Title,                // H1 title
		config.Message,              // Main message
		subMessageHTML,              // Submessage box
		additionalHTML,              // Additional HTML
		closeHint,                   // Close hint
	)
}

// pageTheme contains color scheme and styling for a page type
type pageTheme struct {
	backgroundGradient string
	iconColor          string
	iconSVG            string
	messageBackground  string
	messageTextColor   string
}

// getTheme returns the color scheme for a given page type
func getTheme(pageType PageType) pageTheme {
	switch pageType {
	case PageTypeSuccess:
		return pageTheme{
			backgroundGradient: "linear-gradient(135deg, #34a853 0%, #1a73e8 100%)",
			iconColor:          "#34a853",
			iconSVG: `<svg viewBox="0 0 24 24">
                <polyline points="20 6 9 17 4 12"></polyline>
            </svg>`,
			messageBackground: "#e8f5e9",
			messageTextColor:  "#1e4620",
		}
	case PageTypeError:
		return pageTheme{
			backgroundGradient: "linear-gradient(135deg, #ea4335 0%, #fbbc04 100%)",
			iconColor:          "#ea4335",
			iconSVG: `<svg viewBox="0 0 24 24">
                <circle cx="12" cy="12" r="10"></circle>
                <line x1="15" y1="9" x2="9" y2="15"></line>
                <line x1="9" y1="9" x2="15" y2="15"></line>
            </svg>`,
			messageBackground: "#fce4ec",
			messageTextColor:  "#c62828",
		}
	case PageTypeInfo:
		return pageTheme{
			backgroundGradient: "linear-gradient(135deg, #1a73e8 0%, #34a853 100%)",
			iconColor:          "#1a73e8",
			iconSVG: `<svg viewBox="0 0 24 24">
                <circle cx="12" cy="12" r="10"></circle>
                <line x1="12" y1="8" x2="12" y2="12"></line>
                <line x1="12" y1="16" x2="12.01" y2="16"></line>
            </svg>`,
			messageBackground: "#e3f2fd",
			messageTextColor:  "#1565c0",
		}
	case PageTypeWarning:
		return pageTheme{
			backgroundGradient: "linear-gradient(135deg, #fbbc04 0%, #ea4335 100%)",
			iconColor:          "#fbbc04",
			iconSVG: `<svg viewBox="0 0 24 24">
                <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path>
                <line x1="12" y1="9" x2="12" y2="13"></line>
                <line x1="12" y1="17" x2="12.01" y2="17"></line>
            </svg>`,
			messageBackground: "#fff3cd",
			messageTextColor:  "#856404",
		}
	case PageTypePending:
		return pageTheme{
			backgroundGradient: "linear-gradient(135deg, #9c27b0 0%, #1a73e8 100%)",
			iconColor:          "#9c27b0",
			iconSVG: `<svg viewBox="0 0 24 24">
                <circle cx="12" cy="12" r="10"></circle>
                <polyline points="12 6 12 12 16 14"></polyline>
            </svg>`,
			messageBackground: "#f3e5f5",
			messageTextColor:  "#6a1b9a",
		}
	default:
		// Default to info
		return getTheme(PageTypeInfo)
	}
}
