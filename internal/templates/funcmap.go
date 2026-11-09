package templates

import (
	"fmt"
	"html/template"
	"regexp"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
)

var nonAlphaNumRegex = regexp.MustCompile(`[^a-z0-9]+`)

// SafeFuncMap creates an allowlisted, side-effect-free template.FuncMap for themes.
func SafeFuncMap(activeThemeSlug string) template.FuncMap {
	sanitizer := bluemonday.UGCPolicy()

	return template.FuncMap{
		// 1. Security: Sanitize user HTML with bluemonday before template.HTML cast
		"sanitizeHTML": func(s any) template.HTML {
			str := fmt.Sprint(s)
			clean := sanitizer.Sanitize(str)
			return template.HTML(clean)
		},

		// 2. Safe string manipulation
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"trim":  strings.TrimSpace,
		"contains": func(s, substr string) bool {
			return strings.Contains(s, substr)
		},
		"hasPrefix": func(s, prefix string) bool {
			return strings.HasPrefix(s, prefix)
		},
		"truncate": func(s string, maxLen int) string {
			runes := []rune(s)
			if len(runes) <= maxLen {
				return s
			}
			return string(runes[:maxLen]) + "..."
		},
		"slugify": func(s string) string {
			lower := strings.ToLower(strings.TrimSpace(s))
			clean := nonAlphaNumRegex.ReplaceAllString(lower, "-")
			return strings.Trim(clean, "-")
		},

		// 3. Date & Time formatting
		"formatDate": func(t any, layout string) string {
			if layout == "" {
				layout = "Jan 02, 2006"
			}
			switch v := t.(type) {
			case time.Time:
				return v.Format(layout)
			case *time.Time:
				if v != nil {
					return v.Format(layout)
				}
				return ""
			case string:
				parsed, err := time.Parse(time.RFC3339, v)
				if err == nil {
					return parsed.Format(layout)
				}
				parsedDate, err := time.Parse("2006-01-02", v)
				if err == nil {
					return parsedDate.Format(layout)
				}
				return v
			default:
				return ""
			}
		},
		"timeAgo": func(t any) string {
			var dt time.Time
			switch v := t.(type) {
			case time.Time:
				dt = v
			case *time.Time:
				if v != nil {
					dt = *v
				} else {
					return ""
				}
			case string:
				var err error
				dt, err = time.Parse(time.RFC3339, v)
				if err != nil {
					return v
				}
			default:
				return ""
			}

			diff := time.Since(dt)
			if diff < time.Minute {
				return "just now"
			} else if diff < time.Hour {
				mins := int(diff.Minutes())
				if mins == 1 {
					return "1 minute ago"
				}
				return fmt.Sprintf("%d minutes ago", mins)
			} else if diff < 24*time.Hour {
				hrs := int(diff.Hours())
				if hrs == 1 {
					return "1 hour ago"
				}
				return fmt.Sprintf("%d hours ago", hrs)
			} else {
				days := int(diff.Hours() / 24)
				if days == 1 {
					return "yesterday"
				}
				return fmt.Sprintf("%d days ago", days)
			}
		},

		// 4. Number & Currency formatting
		"formatNumber": func(num any) string {
			return fmt.Sprintf("%v", num)
		},
		"formatCurrency": func(val any, symbol string) string {
			if symbol == "" {
				symbol = "$"
			}
			switch v := val.(type) {
			case float64:
				return fmt.Sprintf("%s%.2f", symbol, v)
			case int:
				return fmt.Sprintf("%s%d.00", symbol, v)
			case int64:
				return fmt.Sprintf("%s%d.00", symbol, v)
			default:
				return fmt.Sprintf("%s%v", symbol, v)
			}
		},

		// 5. Arithmetic & Math helpers
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},

		// 6. Template logic utilities
		"defaultVal": func(val, defaultVal any) any {
			if val == nil || val == "" {
				return defaultVal
			}
			return val
		},
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict requires an even number of key/value arguments")
			}
			dict := make(map[string]any, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},

		// 7. Safe Asset URL generator
		"assetPath": func(relPath string) string {
			cleanPath := strings.TrimPrefix(relPath, "/")
			cleanPath = strings.ReplaceAll(cleanPath, "..", "")
			return fmt.Sprintf("/themes/%s/assets/%s", activeThemeSlug, cleanPath)
		},
	}
}
