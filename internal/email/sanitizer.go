package email

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var (
	emailHTMLPolicy = buildEmailHTMLPolicy()
	varPlaceholder  = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)
)

func buildEmailHTMLPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowElements("div", "span", "table", "thead", "tbody", "tr", "td", "th", "hr", "center")
	p.AllowAttrs("style", "align", "width", "cellpadding", "cellspacing", "border").OnElements("table", "tr", "td", "th", "div", "p", "span", "a", "h1", "h2", "h3", "h4")
	p.AllowURLSchemes("http", "https", "mailto")
	return p
}

// SanitizeHTML cleans untrusted markup in email templates to strip scripts and dangerous handlers.
func SanitizeHTML(raw string) string {
	return strings.TrimSpace(emailHTMLPolicy.Sanitize(raw))
}

// HasHeaderInjection checks whether s contains CR/LF sequences that could inject SMTP headers.
func HasHeaderInjection(s string) bool {
	return strings.ContainsAny(s, "\r\n")
}

func stripLineBreaks(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func substituteVars(tmpl string, vars map[string]string, escapeHTML bool) string {
	return varPlaceholder.ReplaceAllStringFunc(tmpl, func(match string) string {
		sub := varPlaceholder.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		val, ok := vars[sub[1]]
		if !ok {
			return ""
		}
		if escapeHTML {
			return html.EscapeString(val)
		}
		return val
	})
}

// RenderTemplate interpolates variables safely and sanitizes resulting HTML and header strings.
func RenderTemplate(tmpl EmailTemplate, vars map[string]string) (PreviewResult, error) {
	if HasHeaderInjection(tmpl.FromName) || HasHeaderInjection(tmpl.FromEmail) || HasHeaderInjection(tmpl.ReplyTo) {
		return PreviewResult{}, fmt.Errorf("template header fields contain illegal newline characters")
	}

	safeHeaderVars := make(map[string]string, len(vars))
	for k, v := range vars {
		safeHeaderVars[k] = stripLineBreaks(v)
	}

	subject := stripLineBreaks(substituteVars(tmpl.Subject, safeHeaderVars, false))
	renderedHTML := SanitizeHTML(substituteVars(tmpl.BodyHTML, vars, true))
	renderedText := substituteVars(tmpl.BodyText, vars, false)

	return PreviewResult{
		Subject:  subject,
		BodyHTML: renderedHTML,
		BodyText: renderedText,
	}, nil
}

