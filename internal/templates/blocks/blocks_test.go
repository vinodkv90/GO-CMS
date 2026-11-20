package blocks

import (
	"context"
	"strings"
	"testing"
)

type mockContentProvider struct {
	items []map[string]any
}

func (m *mockContentProvider) ListPublished(ctx context.Context, collectionUID string, limit int, preview bool) ([]map[string]any, error) {
	return m.items, nil
}

func TestParseBlocks(t *testing.T) {
	rawJSON := `[
		{
			"id": "b1",
			"type": "core/heading",
			"attributes": {"level": 1, "content": "Hello World"}
		},
		{
			"id": "b2",
			"type": "core/paragraph",
			"attributes": {"content": "This is a paragraph."}
		}
	]`

	blocks, err := ParseBlocks(rawJSON)
	if err != nil {
		t.Fatalf("unexpected error parsing blocks: %v", err)
	}

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}

	if blocks[0].Type != "core/heading" {
		t.Errorf("expected core/heading, got %s", blocks[0].Type)
	}
	if blocks[1].Type != "core/paragraph" {
		t.Errorf("expected core/paragraph, got %s", blocks[1].Type)
	}
}

func TestRenderBlocks_CoreBlocks(t *testing.T) {
	tree := []Block{
		{
			ID:   "sec-1",
			Type: "core/section",
			Attributes: map[string]any{
				"padding":         "py-16",
				"backgroundColor": "#f8fafc",
			},
			Children: []Block{
				{
					ID:   "cont-1",
					Type: "core/container",
					Attributes: map[string]any{
						"maxWidth": "max-w-4xl",
					},
					Children: []Block{
						{
							ID:   "h1",
							Type: "core/heading",
							Attributes: map[string]any{
								"level":   1,
								"content": "Welcome to Go CMS",
								"align":   "center",
							},
						},
						{
							ID:   "p1",
							Type: "core/paragraph",
							Attributes: map[string]any{
								"content": "Visual Page Builder built for speed.",
								"align":   "center",
							},
						},
						{
							ID:   "btn1",
							Type: "core/button",
							Attributes: map[string]any{
								"text":    "Get Started",
								"url":     "/about-us",
								"variant": "primary",
								"newTab":  false,
							},
						},
					},
				},
			},
		},
	}

	rCtx := NewRenderContext(context.Background(), nil, false)
	html := string(RenderBlocks(tree, rCtx))

	if !strings.Contains(html, "<section class=\"cms-block-section py-16") {
		t.Errorf("expected section element with py-16, got %s", html)
	}
	if !strings.Contains(html, "background-color: #f8fafc") {
		t.Errorf("expected background-color style, got %s", html)
	}
	if !strings.Contains(html, "<h1 class=\"cms-block-heading cms-h1 text-center\"") {
		t.Errorf("expected h1 heading with text-center, got %s", html)
	}
	if !strings.Contains(html, "Welcome to Go CMS") {
		t.Errorf("expected heading content, got %s", html)
	}
	if !strings.Contains(html, "<a href=\"/about-us\" class=\"cms-btn cms-btn-primary\"") {
		t.Errorf("expected button with link, got %s", html)
	}
}

func TestRenderBlocks_Sanitization(t *testing.T) {
	tree := []Block{
		{
			ID:   "xss-h",
			Type: "core/heading",
			Attributes: map[string]any{
				"level":   2,
				"content": "Title <script>alert('xss')</script>",
			},
		},
		{
			ID:   "xss-btn",
			Type: "core/button",
			Attributes: map[string]any{
				"text": "Click",
				"url":  "javascript:alert('xss')",
			},
		},
	}

	rCtx := NewRenderContext(context.Background(), nil, false)
	html := string(RenderBlocks(tree, rCtx))

	if strings.Contains(html, "<script>") {
		t.Errorf("bluemonday failed to remove script tag: %s", html)
	}
	if strings.Contains(html, "javascript:") {
		t.Errorf("failed to sanitize dangerous javascript URL: %s", html)
	}
}

func TestRenderBlocks_DynamicPosts(t *testing.T) {
	mockProv := &mockContentProvider{
		items: []map[string]any{
			{
				"title":        "First Article",
				"_slug":        "first-article",
				"_type":        "blog-posts",
				"published_at": "2026-11-20",
				"excerpt":      "Summary of first article.",
			},
		},
	}

	tree := []Block{
		{
			ID:   "dyn-1",
			Type: "core/dynamic-posts",
			Attributes: map[string]any{
				"collectionUid": "blog-posts",
				"layout":        "grid",
			},
		},
	}

	rCtx := NewRenderContext(context.Background(), mockProv, false)
	html := string(RenderBlocks(tree, rCtx))

	if !strings.Contains(html, "First Article") {
		t.Errorf("expected First Article in dynamic posts, got %s", html)
	}
	if !strings.Contains(html, "/blog-posts/first-article") {
		t.Errorf("expected link /blog-posts/first-article, got %s", html)
	}
}

