package templates

import "time"

// SiteInfo holds global metadata about the website.
type SiteInfo struct {
	Title         string         `json:"title"`
	Tagline       string         `json:"tagline"`
	BaseURL       string         `json:"base_url"`
	Language      string         `json:"language"`
	ThemeSettings map[string]any `json:"theme_settings"`
}

// SEOMetadata provides search engine and social media optimization tags.
type SEOMetadata struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
	Canonical   string   `json:"canonical"`
	OGImage     string   `json:"og_image"`
	OGType      string   `json:"og_type"`
	TwitterCard string   `json:"twitter_card"`
	Robots      string   `json:"robots"`
}

// BreadcrumbItem represents an entry in the navigation breadcrumbs trail.
type BreadcrumbItem struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Pagination holds pagination state and links for archive listings.
type Pagination struct {
	CurrentPage int    `json:"current_page"`
	TotalPages  int    `json:"total_pages"`
	TotalItems  int    `json:"total_items"`
	PerPage     int    `json:"per_page"`
	HasPrev     bool   `json:"has_prev"`
	HasNext     bool   `json:"has_next"`
	PrevURL     string `json:"prev_url"`
	NextURL     string `json:"next_url"`
}

// TemplateData is the strongly typed context injected into Go html/template execution.
type TemplateData struct {
	Site        SiteInfo
	SEO         SEOMetadata
	Breadcrumbs []BreadcrumbItem
	Pagination  *Pagination
	Page        map[string]any
	Posts       []map[string]any
	Entry       map[string]any
	Params      map[string]string
	CSRFToken   string
	Preview     bool
	GeneratedAt time.Time
}

