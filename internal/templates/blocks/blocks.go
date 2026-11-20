package blocks

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"net/url"
	"strconv"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// Block represents a single visual block in the page builder tree.
type Block struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Attributes  map[string]any `json:"attributes"`
	Children    []Block        `json:"children,omitempty"`
	InnerBlocks []Block        `json:"innerBlocks,omitempty"`
}

// GetChildren returns the nested blocks, checking both Children and InnerBlocks.
func (b Block) GetChildren() []Block {
	if len(b.Children) > 0 {
		return b.Children
	}
	return b.InnerBlocks
}

// ContentProvider abstracts dynamic CMS data fetching for dynamic blocks.
type ContentProvider interface {
	ListPublished(ctx context.Context, collectionUID string, limit int, preview bool) ([]map[string]any, error)
}

// RenderContext passes execution context and options to block renderers.
type RenderContext struct {
	Ctx             context.Context
	ContentProvider ContentProvider
	Preview         bool
	Sanitizer       *bluemonday.Policy
}

// NewRenderContext initializes a standard rendering context with Bluemonday UGC sanitization.
func NewRenderContext(ctx context.Context, provider ContentProvider, preview bool) *RenderContext {
	if ctx == nil {
		ctx = context.Background()
	}
	return &RenderContext{
		Ctx:             ctx,
		ContentProvider: provider,
		Preview:         preview,
		Sanitizer:       bluemonday.UGCPolicy(),
	}
}

// ParseBlocks parses a JSON raw string or []any into a slice of Block structs.
func ParseBlocks(raw any) ([]Block, error) {
	if raw == nil {
		return nil, nil
	}

	switch v := raw.(type) {
	case []Block:
		return v, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		var blocks []Block
		if err := json.Unmarshal([]byte(v), &blocks); err != nil {
			return nil, fmt.Errorf("failed to parse block json: %w", err)
		}
		return blocks, nil
	case []byte:
		if len(v) == 0 {
			return nil, nil
		}
		var blocks []Block
		if err := json.Unmarshal(v, &blocks); err != nil {
			return nil, fmt.Errorf("failed to parse block json: %w", err)
		}
		return blocks, nil
	default:
		// Re-encode via JSON to handle []interface{} from map[string]any
		bytes, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal raw block data: %w", err)
		}
		var blocks []Block
		if err := json.Unmarshal(bytes, &blocks); err != nil {
			return nil, fmt.Errorf("failed to unmarshal into []Block: %w", err)
		}
		return blocks, nil
	}
}

// RenderBlocks recursively renders a list of blocks into template.HTML.
func RenderBlocks(blocks []Block, rCtx *RenderContext) template.HTML {
	if len(blocks) == 0 {
		return ""
	}
	if rCtx == nil {
		rCtx = NewRenderContext(context.Background(), nil, false)
	}

	var sb strings.Builder
	for _, block := range blocks {
		sb.WriteString(renderSingleBlock(block, rCtx))
	}
	return template.HTML(sb.String())
}

func renderSingleBlock(b Block, rCtx *RenderContext) string {
	attrs := b.Attributes
	if attrs == nil {
		attrs = make(map[string]any)
	}

	switch b.Type {
	case "core/section":
		return renderSection(b, attrs, rCtx)
	case "core/container":
		return renderContainer(b, attrs, rCtx)
	case "core/columns":
		return renderColumns(b, attrs, rCtx)
	case "core/column":
		return renderColumn(b, attrs, rCtx)
	case "core/heading":
		return renderHeading(attrs, rCtx)
	case "core/paragraph":
		return renderParagraph(attrs, rCtx)
	case "core/image":
		return renderImage(attrs, rCtx)
	case "core/button":
		return renderButton(attrs, rCtx)
	case "core/separator":
		return renderSeparator(attrs)
	case "core/spacer":
		return renderSpacer(attrs)
	case "core/quote":
		return renderQuote(attrs, rCtx)
	case "core/dynamic-posts":
		return renderDynamicPosts(attrs, rCtx)
	default:
		// Fallback for custom or unrecognized block types: render children if any
		children := b.GetChildren()
		if len(children) > 0 {
			var sb strings.Builder
			for _, child := range children {
				sb.WriteString(renderSingleBlock(child, rCtx))
			}
			return sb.String()
		}
		return ""
	}
}

func renderSection(b Block, attrs map[string]any, rCtx *RenderContext) string {
	padding := getString(attrs, "padding", "py-12")
	bgColor := getString(attrs, "backgroundColor", "")
	textColor := getString(attrs, "textColor", "")
	customClass := getString(attrs, "customClass", "")

	var styleParts []string
	if bgColor != "" {
		styleParts = append(styleParts, "background-color: "+sanitizeColor(bgColor))
	}
	if textColor != "" {
		styleParts = append(styleParts, "color: "+sanitizeColor(textColor))
	}

	styleAttr := ""
	if len(styleParts) > 0 {
		styleAttr = fmt.Sprintf(` style="%s;"`, html.EscapeString(strings.Join(styleParts, "; ")))
	}

	var childrenHTML strings.Builder
	for _, child := range b.GetChildren() {
		childrenHTML.WriteString(renderSingleBlock(child, rCtx))
	}

	return fmt.Sprintf(
		`<section class="cms-block-section %s %s"%s><div class="cms-section-inner">%s</div></section>`,
		html.EscapeString(padding),
		html.EscapeString(customClass),
		styleAttr,
		childrenHTML.String(),
	)
}

func renderContainer(b Block, attrs map[string]any, rCtx *RenderContext) string {
	maxWidth := getString(attrs, "maxWidth", "max-w-5xl")
	padding := getString(attrs, "padding", "px-4")

	var childrenHTML strings.Builder
	for _, child := range b.GetChildren() {
		childrenHTML.WriteString(renderSingleBlock(child, rCtx))
	}

	return fmt.Sprintf(
		`<div class="cms-block-container %s mx-auto %s">%s</div>`,
		html.EscapeString(maxWidth),
		html.EscapeString(padding),
		childrenHTML.String(),
	)
}

func renderColumns(b Block, attrs map[string]any, rCtx *RenderContext) string {
	cols := getInt(attrs, "columns", 2)
	gap := getString(attrs, "gap", "gap-6")

	colsClass := "cms-grid-cols-2"
	switch cols {
	case 1:
		colsClass = "cms-grid-cols-1"
	case 3:
		colsClass = "cms-grid-cols-3"
	case 4:
		colsClass = "cms-grid-cols-4"
	}

	var childrenHTML strings.Builder
	for _, child := range b.GetChildren() {
		childrenHTML.WriteString(renderSingleBlock(child, rCtx))
	}

	return fmt.Sprintf(
		`<div class="cms-block-columns grid %s %s">%s</div>`,
		html.EscapeString(colsClass),
		html.EscapeString(gap),
		childrenHTML.String(),
	)
}

func renderColumn(b Block, attrs map[string]any, rCtx *RenderContext) string {
	width := getString(attrs, "width", "w-full")

	var childrenHTML strings.Builder
	for _, child := range b.GetChildren() {
		childrenHTML.WriteString(renderSingleBlock(child, rCtx))
	}

	return fmt.Sprintf(
		`<div class="cms-block-column %s">%s</div>`,
		html.EscapeString(width),
		childrenHTML.String(),
	)
}

func renderHeading(attrs map[string]any, rCtx *RenderContext) string {
	level := getInt(attrs, "level", 2)
	if level < 1 || level > 6 {
		level = 2
	}

	rawContent := getString(attrs, "content", "")
	cleanContent := rCtx.Sanitizer.Sanitize(rawContent)
	if cleanContent == "" {
		cleanContent = html.EscapeString(rawContent)
	}

	align := getString(attrs, "align", "left")
	alignClass := "text-" + align

	textColor := getString(attrs, "textColor", "")
	styleAttr := ""
	if textColor != "" {
		styleAttr = fmt.Sprintf(` style="color: %s;"`, html.EscapeString(sanitizeColor(textColor)))
	}

	return fmt.Sprintf(
		`<h%d class="cms-block-heading cms-h%d %s"%s>%s</h%d>`,
		level, level, html.EscapeString(alignClass), styleAttr, cleanContent, level,
	)
}

func renderParagraph(attrs map[string]any, rCtx *RenderContext) string {
	rawContent := getString(attrs, "content", "")
	cleanContent := rCtx.Sanitizer.Sanitize(rawContent)
	if cleanContent == "" {
		cleanContent = html.EscapeString(rawContent)
	}

	align := getString(attrs, "align", "left")
	alignClass := "text-" + align

	textColor := getString(attrs, "textColor", "")
	styleAttr := ""
	if textColor != "" {
		styleAttr = fmt.Sprintf(` style="color: %s;"`, html.EscapeString(sanitizeColor(textColor)))
	}

	return fmt.Sprintf(
		`<div class="cms-block-paragraph %s"%s>%s</div>`,
		html.EscapeString(alignClass), styleAttr, cleanContent,
	)
}

func renderImage(attrs map[string]any, rCtx *RenderContext) string {
	rawURL := getString(attrs, "url", "")
	cleanURL := sanitizeURL(rawURL)
	if cleanURL == "" {
		return ""
	}

	alt := getString(attrs, "alt", "")
	caption := getString(attrs, "caption", "")
	align := getString(attrs, "align", "center")
	alignClass := "text-" + align

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<figure class="cms-block-image %s">`, html.EscapeString(alignClass)))
	sb.WriteString(fmt.Sprintf(
		`<img src="%s" alt="%s" class="cms-img-responsive rounded-lg shadow-sm" loading="lazy" />`,
		html.EscapeString(cleanURL),
		html.EscapeString(alt),
	))
	if caption != "" {
		cleanCaption := rCtx.Sanitizer.Sanitize(caption)
		sb.WriteString(fmt.Sprintf(`<figcaption class="cms-img-caption text-sm text-slate-500 mt-2">%s</figcaption>`, cleanCaption))
	}
	sb.WriteString(`</figure>`)
	return sb.String()
}

func renderButton(attrs map[string]any, rCtx *RenderContext) string {
	text := getString(attrs, "text", "Click Here")
	rawURL := getString(attrs, "url", "#")
	cleanURL := sanitizeURL(rawURL)
	if cleanURL == "" {
		cleanURL = "#"
	}

	variant := getString(attrs, "variant", "primary")
	align := getString(attrs, "align", "left")
	alignClass := "text-" + align
	newTab := getBool(attrs, "newTab", false)

	targetAttr := ""
	if newTab {
		targetAttr = ` target="_blank" rel="noopener noreferrer"`
	}

	cleanText := rCtx.Sanitizer.Sanitize(text)
	if cleanText == "" {
		cleanText = html.EscapeString(text)
	}

	return fmt.Sprintf(
		`<div class="cms-block-button-wrapper %s"><a href="%s" class="cms-btn cms-btn-%s"%s>%s</a></div>`,
		html.EscapeString(alignClass),
		html.EscapeString(cleanURL),
		html.EscapeString(variant),
		targetAttr,
		cleanText,
	)
}

func renderSeparator(attrs map[string]any) string {
	style := getString(attrs, "style", "solid")
	return fmt.Sprintf(`<hr class="cms-block-separator cms-separator-%s" />`, html.EscapeString(style))
}

func renderSpacer(attrs map[string]any) string {
	height := getString(attrs, "height", "32px")
	if !strings.HasSuffix(height, "px") && !strings.HasSuffix(height, "rem") {
		height = height + "px"
	}
	return fmt.Sprintf(`<div class="cms-block-spacer" style="height: %s;"></div>`, html.EscapeString(height))
}

func renderQuote(attrs map[string]any, rCtx *RenderContext) string {
	content := getString(attrs, "content", "")
	citation := getString(attrs, "citation", "")

	cleanContent := rCtx.Sanitizer.Sanitize(content)
	cleanCitation := rCtx.Sanitizer.Sanitize(citation)

	var sb strings.Builder
	sb.WriteString(`<blockquote class="cms-block-quote border-l-4 border-sky-500 pl-4 py-2 italic my-4 text-slate-700">`)
	sb.WriteString(fmt.Sprintf(`<p>%s</p>`, cleanContent))
	if cleanCitation != "" {
		sb.WriteString(fmt.Sprintf(`<cite class="block text-sm not-italic text-slate-500 mt-2">— %s</cite>`, cleanCitation))
	}
	sb.WriteString(`</blockquote>`)
	return sb.String()
}

func renderDynamicPosts(attrs map[string]any, rCtx *RenderContext) string {
	collectionUID := getString(attrs, "collectionUid", "blog-posts")
	limit := getInt(attrs, "limit", 6)
	layout := getString(attrs, "layout", "grid")
	showDate := getBool(attrs, "showDate", true)
	showExcerpt := getBool(attrs, "showExcerpt", true)

	if rCtx.ContentProvider == nil {
		return `<div class="cms-dynamic-empty text-slate-400 italic py-4">Dynamic posts unavailable.</div>`
	}

	posts, err := rCtx.ContentProvider.ListPublished(rCtx.Ctx, collectionUID, limit, rCtx.Preview)
	if err != nil || len(posts) == 0 {
		return `<div class="cms-dynamic-empty text-slate-400 italic py-4">No published items found.</div>`
	}

	var sb strings.Builder
	if layout == "list" {
		sb.WriteString(`<div class="cms-posts-list space-y-4">`)
		for _, p := range posts {
			title := getString(p, "title", "Untitled")
			slug := getString(p, "_slug", getString(p, "slug", ""))
			postType := getString(p, "_type", collectionUID)
			dateStr := getString(p, "published_at", "")
			excerpt := getString(p, "excerpt", "")

			sb.WriteString(`<article class="cms-post-item border-b border-slate-200 pb-3">`)
			sb.WriteString(fmt.Sprintf(`<h3 class="font-bold text-lg"><a href="/%s/%s" class="text-sky-600 hover:underline">%s</a></h3>`,
				html.EscapeString(postType), html.EscapeString(slug), html.EscapeString(title)))
			if showDate && dateStr != "" {
				sb.WriteString(fmt.Sprintf(`<time class="text-xs text-slate-500 block mb-1">%s</time>`, html.EscapeString(dateStr)))
			}
			if showExcerpt && excerpt != "" {
				sb.WriteString(fmt.Sprintf(`<p class="text-sm text-slate-600">%s</p>`, html.EscapeString(excerpt)))
			}
			sb.WriteString(`</article>`)
		}
		sb.WriteString(`</div>`)
	} else {
		sb.WriteString(`<div class="cms-posts-grid grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 my-6">`)
		for _, p := range posts {
			title := getString(p, "title", "Untitled")
			slug := getString(p, "_slug", getString(p, "slug", ""))
			postType := getString(p, "_type", collectionUID)
			dateStr := getString(p, "published_at", "")
			excerpt := getString(p, "excerpt", "")

			sb.WriteString(`<article class="cms-post-card bg-white p-5 rounded-xl border border-slate-200 shadow-sm flex flex-col justify-between">`)
			sb.WriteString(`<div>`)
			sb.WriteString(fmt.Sprintf(`<h3 class="font-bold text-base mb-2"><a href="/%s/%s" class="text-slate-900 hover:text-sky-600">%s</a></h3>`,
				html.EscapeString(postType), html.EscapeString(slug), html.EscapeString(title)))
			if showDate && dateStr != "" {
				sb.WriteString(fmt.Sprintf(`<time class="text-xs text-slate-400 block mb-2">%s</time>`, html.EscapeString(dateStr)))
			}
			if showExcerpt && excerpt != "" {
				sb.WriteString(fmt.Sprintf(`<p class="text-sm text-slate-600 mb-4 line-clamp-3">%s</p>`, html.EscapeString(excerpt)))
			}
			sb.WriteString(`</div>`)
			sb.WriteString(fmt.Sprintf(`<a href="/%s/%s" class="text-xs font-semibold text-sky-600 hover:text-sky-700">Read more &rarr;</a>`,
				html.EscapeString(postType), html.EscapeString(slug)))
			sb.WriteString(`</article>`)
		}
		sb.WriteString(`</div>`)
	}

	return sb.String()
}

// Helpers

func getString(m map[string]any, key, defaultVal string) string {
	if m == nil {
		return defaultVal
	}
	if v, ok := m[key]; ok && v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return defaultVal
}

func getInt(m map[string]any, key string, defaultVal int) int {
	if m == nil {
		return defaultVal
	}
	if v, ok := m[key]; ok && v != nil {
		switch num := v.(type) {
		case int:
			return num
		case int64:
			return int(num)
		case float64:
			return int(num)
		case string:
			if parsed, err := strconv.Atoi(num); err == nil {
				return parsed
			}
		}
	}
	return defaultVal
}

func getBool(m map[string]any, key string, defaultVal bool) bool {
	if m == nil {
		return defaultVal
	}
	if v, ok := m[key]; ok && v != nil {
		if b, ok := v.(bool); ok {
			return b
		}
		if s, ok := v.(string); ok {
			return strings.EqualFold(s, "true") || s == "1"
		}
	}
	return defaultVal
}

func sanitizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Allow root-relative paths like /uploads/img.png
	if strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "./") {
		return raw
	}
	// Allow hash anchors
	if strings.HasPrefix(raw, "#") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto", "tel":
		return raw
	default:
		return ""
	}
}

func sanitizeColor(raw string) string {
	raw = strings.TrimSpace(raw)
	// Simple validation for hex, rgb, rgba, or CSS color names
	if strings.HasPrefix(raw, "#") {
		clean := strings.TrimPrefix(raw, "#")
		if len(clean) == 3 || len(clean) == 6 || len(clean) == 8 {
			return raw
		}
	}
	if strings.HasPrefix(raw, "rgb(") || strings.HasPrefix(raw, "rgba(") || strings.HasPrefix(raw, "hsl(") {
		if strings.HasSuffix(raw, ")") && !strings.ContainsAny(raw, ";\"'<>") {
			return raw
		}
	}
	// Simple alpha name
	isAlpha := true
	for _, r := range raw {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '-' {
			isAlpha = false
			break
		}
	}
	if isAlpha && len(raw) <= 30 {
		return raw
	}
	return ""
}

