# Go CMS Installation & Quickstart Guide

Go CMS is a modern open-source content management system offering the flexibility of **Headless CMS APIs** alongside the visual page-building power of **Templated CMS (Go HTML templates + Gutenberg blocks)**.

---

## 1. Quickstart with `npx` (Recommended)

Just like Next.js (`create-next-app`) and Strapi (`create-strapi-app`), you can scaffold and run a complete Go CMS project with a single command:

```bash
npx create-go-cms@latest my-project
cd my-project
npm run dev
```

### What Happens:
1. `create-go-cms` detects your operating system (Windows, Linux, macOS) and architecture (`amd64`, `arm64`).
2. Guides you through an interactive setup (or use `--quickstart` for instant defaults).
3. Configures a project directory with `config.yaml`, secure 64-character random secrets in `.env`, and working npm scripts.
4. If **Templated CMS mode** is selected, extracts the default theme and Gutenberg visual block templates.
5. Verifies and provisions the native `go-cms` binary.

---

## 2. Native Go CLI Installation

If you prefer using pure Go tools without Node.js:

### Install the CLI:
```bash
go install github.com/go-cms/cms/cmd/go-cms@latest
```

### Create and Run Your Project:
```bash
go-cms create my-project
cd my-project
go-cms develop
```

---

## 3. Choosing Your CMS Mode

When creating a new project, Go CMS supports two application architectures:

### Mode 1: Templated CMS (Server-Side Rendering)
* **Best for:** Complete websites, corporate portals, blogs, portfolios, and marketing sites.
* **Key Features:**
  * Go standard library `html/template` engine with blazing-fast server-side rendering (<2ms TTFB).
  * 3-panel Gutenberg Visual Page Builder (Sections, Columns, Dynamic Post Lists, Buttons, Headings).
  * Live responsive viewport preview (Desktop, Tablet 768px, Mobile 375px).
  * Built-in themes in `./themes/` with full layout, partial, and asset customization.

### Mode 2: Headless CMS (API-First)
* **Best for:** Jamstack sites (Next.js, Nuxt, Astro), mobile apps, or microservice architectures.
* **Key Features:**
  * JSON REST APIs under `/api/v1/content/*`.
  * Multi-stage publishing (Draft vs. Published), version history, optimistic locking.
  * API Tokens, Transfer Tokens, and RBAC permissions.
  * Media Library with multi-provider cloud storage (S3, MinIO, Cloudinary, Azure, GCS, Local).

---

## 4. Project Structure

A generated Go CMS project is clean, self-contained, and version-controlled:

```text
my-project/
├── config.yaml          # Main configuration (server, DB, storage, auth, logging)
├── .env                 # Environment secrets and overrides
├── package.json         # Working scripts: dev, start, build, migrate, doctor
├── .gitignore           # Git ignore rules for data, logs, and secrets
├── README.md            # Project documentation and quick reference
├── bin/                 # Local pinned native executable (go-cms)
├── scripts/             # Cross-platform runner script
├── schemas/             # Custom declarative content type definitions
├── themes/              # (Templated mode) Theme layouts, templates, and CSS
│   └── default/
│       ├── theme.json
│       ├── assets/
│       └── templates/
├── plugins/             # Native Go plugins and extension modules
└── uploads/             # Locally stored media files
```

---

## 5. First-Run Setup & Default Credentials

On initial startup (`npm run dev` or `go-cms develop`), Go CMS automatically:
1. Connects to the database and applies pending schema migrations.
2. Bootstraps default Administrator roles and permissions.
3. Provisions the default Super Administrator account:
   * **Login URL:** [http://localhost:8080/admin/](http://localhost:8080/admin/)
   * **Username:** `admin`
   * **Password:** `Admin@CMS2026!`
4. Bootstraps the initial Home Page (`/`) and About Us page (`/about-us`) for Templated CMS projects.

> [!IMPORTANT]
> Change the default password upon initial login in **Settings &rarr; Users & Access &rarr; Administrators**.

---

## 6. Available Scripts

Inside any scaffolded project:

| Script | Command | Description |
| :--- | :--- | :--- |
| `npm run dev` | `go-cms develop` | Runs the server in development mode with auto-migrations. |
| `npm run build` | `go-cms build` | Validates templates, schemas, and configuration for production. |
| `npm run start` | `go-cms start` | Runs the server in hardened production mode with caching enabled. |
| `npm run migrate` | `go-cms migrate up` | Applies pending database migrations. |
| `npm run doctor` | `go-cms doctor` | Runs health diagnostics on database, port, and security keys. |

---

## 7. Production Deployment

### Docker Deployment
Run Go CMS in Docker using the official multi-stage container:

```bash
docker run -d \
  --name my-cms \
  -p 8080:8080 \
  -e CMS_SERVER_ENV=production \
  -e CMS_DB_DRIVER=postgres \
  -e CMS_DB_HOST=postgres-host \
  -e CMS_DB_PASSWORD=secret \
  -e CMS_AUTH_JWT_SECRET=production-secret \
  ghcr.io/go-cms/cms:latest
```

### Self-Hosted Binary (systemd)
1. Build project assets: `npm run build`.
2. Move executable to `/usr/local/bin/go-cms`.
3. Create a systemd service pointing to your project directory.
4. Execute `go-cms start --config /etc/go-cms/config.yaml`.

