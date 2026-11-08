# Native HTML Template Engine & Theme Architecture

## 1. Core Principle: Standard Library `html/template`

The Traditional CMS rendering engine is built **strictly using Go's standard library `html/template`**.
- No custom template syntax or foreign DSLs.
- Automatic context-aware escaping is preserved across HTML, JS, CSS, and URI contexts.
- Developers author themes using standard HTML5, CSS, and JavaScript.

---

## 2. Directory Layout & Theme Manifest

Themes are placed under the `themes/` directory. Each theme is fully isolated and portable:

```text
themes/
└── default/
    ├── theme.json               # Mandatory theme metadata & configuration
    ├── assets/                  # Public static assets (CSS, JS, images, fonts)
    │   ├── css/
    │   │   └── style.css
    │   └── js/
    │       └── main.js
    ├── layouts/                 # Root wrapper layouts
    │   ├── base.html            # Primary site master layout
    │   └── admin-preview.html   # Preview layout for live editing
    ├── pages/                   # Single page templates
    │   ├── home.html
    │   ├── about.html
    │   └── contact.html
    ├── posts/                   # Collection item & archive templates
    │   ├── single.html
    │   └── archive.html
    ├── partials/                # Reusable template components
    │   ├── header.html
    │   ├── footer.html
    │   ├── navigation.html
    │   ├── pagination.html
    │   ├── breadcrumbs.html
    │   └── sidebar.html
    └── errors/                  # Error status templates
        ├── 404.html
        └── 500.html
```

### Theme Manifest (`theme.json`)

```json
{
  "name": "Default Theme",
  "slug": "default",
  "version": "1.0.0",
  "description": "Standard responsive theme for modern content sites",
  "author": "CMS Team",
  "engine": "go-html-template",
  "minimumCmsVersion": "1.0.0",
  "settings": {
    "primaryColor": "#3b82f6",
    "postsPerPage": 10,
    "showAuthor": true
  }
}
```

---

## 3. Strongly-Typed Template Context

Templates receive a strongly typed, documented data structure (`TemplateData`):

```go
package templates

type SiteInfo struct {
    Title       string
    Tagline     string
    BaseURL     string
    Language    string
    ThemeSettings map[string]any
}

type SEOMetadata struct {
    Title       string
    Description string
    Keywords    []string
    Canonical   string
    OGImage     string
    OGType      string
    TwitterCard string
    Robots      string
}

type BreadcrumbItem struct {
    Title string
    URL   string
}

type Pagination struct {
    CurrentPage int
    TotalPages  int
    TotalItems  int
    PerPage     int
    HasPrev     bool
    HasNext     bool
    PrevURL     string
    NextURL     string
}

type TemplateData struct {
    Site        SiteInfo
    SEO         SEOMetadata
    Breadcrumbs []BreadcrumbItem
    Pagination  *Pagination
    Page        map[string]any
    Posts       []map[string]any
    Params      map[string]string
    CSRFToken   string
}
```

---

## 4. Safe Template Function Allowlist

The engine provides an allowlisted, side-effect-free `template.FuncMap`:
- String helpers: `upper`, `lower`, `trim`, `truncate`, `slugify`, `contains`.
- Formatting: `formatDate`, `formatTime`, `formatCurrency`, `formatNumber`.
- Math/Logic: `add`, `sub`, `mul`, `div`, `eq`, `ne`, `defaultVal`.
- Security: `sanitizeHTML` (uses `bluemonday` UGCPolicy to sanitize rich text before wrapping in `template.HTML`).
- Path and asset hashing: `assetPath` (returns cache-busted asset URL `/themes/<slug>/assets/css/style.css?v=<hash>`).

System execution, raw filesystem reads, and network calls are strictly excluded from the `FuncMap`.

---

## 5. Layout Inheritance & Composition

Templates leverage Go template blocks for clean inheritance:

### Layout (`layouts/base.html`)
```html
<!DOCTYPE html>
<html lang="{{.Site.Language}}">
<head>
    <meta charset="UTF-8">
    <title>{{if .SEO.Title}}{{.SEO.Title}} - {{end}}{{.Site.Title}}</title>
    <meta name="description" content="{{.SEO.Description}}">
    <link rel="canonical" href="{{.SEO.Canonical}}">
    <link rel="stylesheet" href="{{assetPath "css/style.css"}}">
    {{block "head" .}}{{end}}
</head>
<body class="site-body">
    {{template "partials/header" .}}
    
    <main class="site-main">
        {{block "content" .}}
            <p>Default content</p>
        {{end}}
    </main>

    {{template "partials/footer" .}}
    <script src="{{assetPath "js/main.js"}}"></script>
    {{block "scripts" .}}{{end}}
</body>
</html>
```

### Page Template (`pages/home.html`)
```html
{{define "content"}}
<div class="container hero">
    <h1>{{.Page.Title}}</h1>
    <div class="prose">
        {{.Page.Content | sanitizeHTML}}
    </div>
</div>

<section class="latest-posts">
    <h2>Recent Articles</h2>
    <div class="grid">
        {{range .Posts}}
            <article class="card">
                <h3><a href="/posts/{{.slug}}">{{.title}}</a></h3>
                <p>{{.excerpt}}</p>
                <time datetime="{{.published_at}}">{{formatDate .published_at "Jan 02, 2006"}}</time>
            </article>
        {{end}}
    </div>
</section>
{{end}}
```

---

## 6. Template Caching & Performance

- **Development Mode (`APP_ENV=development`):** Templates are parsed on every incoming request, enabling instantaneous live reload upon editing HTML/CSS.
- **Production Mode (`APP_ENV=production`):** All layouts, partials, and page templates are pre-parsed into a synchronized, concurrent-safe template cache (`sync.RWMutex`), rendering responses in sub-millisecond execution times.

