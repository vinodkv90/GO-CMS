package fields

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var blockTypes = map[string]bool{"paragraph": true, "heading": true, "bulletList": true, "orderedList": true, "listItem": true, "blockquote": true, "codeBlock": true, "image": true, "text": true}

func isBlockType(name string) bool { return blockTypes[name] }

func validateBlocks(value any, rules ValidationRules) error {
	root, ok := value.(map[string]any)
	if !ok || root["type"] != "doc" {
		return fmt.Errorf("expected a structured blocks document")
	}
	if _, err := normalizeJSON(root, false, MaxDepth); err != nil {
		return err
	}
	children, ok := array(root["content"])
	if !ok {
		return fmt.Errorf("document content must be an array")
	}
	allowed := map[string]bool{}
	for _, name := range rules.AllowedBlocks {
		allowed[name] = true
	}
	count := 0
	var visit func(any, int) error
	visit = func(raw any, depth int) error {
		if depth > MaxDepth {
			return fmt.Errorf("blocks document exceeds depth limit")
		}
		count++
		if count > MaxNodes {
			return fmt.Errorf("blocks document exceeds node limit")
		}
		node, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("block node must be an object")
		}
		kind, ok := node["type"].(string)
		if !ok || !isBlockType(kind) {
			return fmt.Errorf("unsupported block type")
		}
		if len(allowed) > 0 && !allowed[kind] {
			return fmt.Errorf("block type %q is not allowed", kind)
		}
		if text, exists := node["text"]; exists {
			if _, ok := text.(string); !ok {
				return fmt.Errorf("block text must be a string")
			}
		}
		if attrs, exists := node["attrs"]; exists {
			attributes, ok := attrs.(map[string]any)
			if !ok {
				return fmt.Errorf("block attributes must be an object")
			}
			if kind == "image" {
				if src, ok := attributes["src"]; ok {
					value, valid := src.(string)
					if !valid || !safeBlockURL(value, true) {
						return fmt.Errorf("image block has an unsafe source URL")
					}
				}
			}
		}
		if marks, exists := node["marks"]; exists {
			items, ok := array(marks)
			if !ok {
				return fmt.Errorf("block marks must be an array")
			}
			for _, item := range items {
				mark, ok := item.(map[string]any)
				if !ok {
					return fmt.Errorf("block mark must be an object")
				}
				name, ok := mark["type"].(string)
				if !ok || !contains([]string{"bold", "italic", "strike", "underline", "code", "link"}, name) {
					return fmt.Errorf("unsupported block mark")
				}
				if name == "link" {
					attrs, ok := mark["attrs"].(map[string]any)
					href, valid := attrs["href"].(string)
					if !ok || !valid || !safeBlockURL(href, false) {
						return fmt.Errorf("link mark has an unsafe URL")
					}
				}
			}
		}
		if content, exists := node["content"]; exists {
			items, ok := array(content)
			if !ok {
				return fmt.Errorf("block content must be an array")
			}
			for _, item := range items {
				if err := visit(item, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, child := range children {
		if err := visit(child, 1); err != nil {
			return err
		}
	}
	return nil
}

func safeBlockURL(raw string, allowRelative bool) bool {
	if raw == "" || strings.ContainsAny(raw, "\\\r\n\t") {
		return false
	}
	if allowRelative && strings.HasPrefix(raw, "/") {
		return !strings.HasPrefix(raw, "//") && !strings.HasPrefix(raw, "/\\")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Hostname() != ""
	case "mailto":
		return parsed.Host == "" && strings.Contains(parsed.Opaque, "@")
	default:
		return false
	}
}

// SanitizeRichText keeps common author formatting and strips executable markup,
// event attributes and unsafe URL protocols before persistence.
func SanitizeRichText(value string) string {
	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("class").OnElements("p", "h1", "h2", "h3", "h4", "pre", "code")
	policy.AllowAttrs("src", "alt", "title", "width", "height").OnElements("img")
	policy.AllowAttrs("href", "title").OnElements("a")
	policy.RequireNoFollowOnLinks(true)
	return policy.Sanitize(value)
}
