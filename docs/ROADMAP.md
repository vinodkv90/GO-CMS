# Project Implementation Roadmap

This document outlines the 10-phase engineering roadmap for the Extensible Go CMS platform. Development proceeds strictly sequentially; no phase is marked complete until all its acceptance tests pass and security requirements are verified.

---

## Phase 1: Foundation (Current Phase)
**Goal:** Establish the rock-solid foundation of the Go backend, configuration, database abstraction layer, capability system, adapters, migrations, HTTP server, and containerization.

- **Milestone 1.1:** Go project setup (`go.mod`), configuration management with environment variable overrides, structured JSON/Text logging (`log/slog`), and context utilities.
- **Milestone 1.2:** Database interface definition, capability system (`DatabaseCapabilities`), connection pooling, and health checks.
- **Milestone 1.3:** PostgreSQL database adapter (`pgx` / `database/sql`).
- **Milestone 1.4:** MySQL database adapter (`go-sql-driver/mysql`).
- **Milestone 1.5:** MongoDB adapter (`mongo-driver`).
- **Milestone 1.6:** Unified schema migration engine supporting SQL scripts and MongoDB index/collection migrations.
- **Milestone 1.7:** HTTP Server engine using Chi router with standard middleware (request ID, recoverer, real IP, structured access logger, CORS, timeouts).
- **Milestone 1.8:** Health endpoints (`/healthz`, `/readyz`, `/info`), graceful shutdown, Dockerfile & `docker-compose.yml`.
- **Milestone 1.9:** Comprehensive test suite for Phase 1.

---

## Phase 2: Authentication, Authorization & User Management
**Goal:** Deliver enterprise-grade identity, session management, RBAC, and security middleware.

- **Milestone 2.1:** User domain model, password hashing (Argon2id), secure password policies.
- **Milestone 2.2:** Session management, secure HTTP-only cookies, CSRF protection, and token rotation.
- **Milestone 2.3:** Role-Based Access Control (RBAC) with granular resource-level permissions (e.g. `content:read`, `content:write`, `content:publish`).
- **Milestone 2.4:** API Token management with expiration and scoped permissions.
- **Milestone 2.5:** Rate limiting, IP lockout, and security event audit logs.
- **Milestone 2.6:** Auth REST API endpoints (`/api/v1/auth/*`, `/api/v1/users/*`, `/api/v1/roles/*`).

---

## Phase 3: Dynamic Content Type Builder & Content Engine
**Goal:** Allow schema creation and content management dynamically without code changes (Strapi equivalent).

- **Milestone 3.1:** Content schema definition engine (Collection Types & Single Types).
- **Milestone 3.2:** Field type registry: Short text, long text, rich text, number, decimal, boolean, date/time, email, secret, URL, enum, JSON, media, component, repeatable component, UID/slug.
- **Milestone 3.3:** Schema migrations for dynamic content types (safe schema evolution without data loss).
- **Milestone 3.4:** Dynamic Content CRUD service with optimistic concurrency control, draft/published states, and soft deletion.
- **Milestone 3.5:** Relationship engine (1:1, 1:N, N:1, N:M).
- **Milestone 3.6:** Version history, diffing, and rollback capability.
- **Milestone 3.7:** Dynamic REST API generation (`/api/v1/content/:type`, filtering, sorting, pagination, search).

---

## Phase 4: Native HTML Template Engine & Theme System
**Goal:** Provide WordPress-grade server-side HTML rendering using Go standard library `html/template`.

- **Milestone 4.1:** Template rendering engine using strictly Go standard `html/template`.
- **Milestone 4.2:** Safe template function allowlist (formatting, slugify, safe HTML with explicit sanitization).
- **Milestone 4.3:** Theme manifest parser, theme directory scanner, and theme loader.
- **Milestone 4.4:** Layout inheritance, reusable partials, and blocks (`base.html`, `header.html`, `footer.html`).
- **Milestone 4.5:** Dynamic route resolver matching content types/slugs to theme templates.
- **Milestone 4.6:** Template caching (hot-reloading in development, compiled cache in production).
- **Milestone 4.7:** SEO metadata injection, dynamic navigation, breadcrumbs, sitemaps, RSS feeds, and error templates (404/500).

---

## Phase 5: Media Library & Storage Provider System
**Goal:** High-performance, secure media storage and on-the-fly or asynchronous image processing.

- **Milestone 5.1:** Common `StorageProvider` abstraction and Local Filesystem provider.
- **Milestone 5.2:** Cloud providers (AWS S3, MinIO, GCS, Azure Blob Storage).
- **Milestone 5.3:** Filename normalization (spaces to hyphens, path traversal prevention, duplicate handling).
- **Milestone 5.4:** Signed URL generator and validator (HMAC-SHA256, expiration, access tokens).
- **Milestone 5.5:** Image processing pipeline (thumbnails, WebP conversion, resizing, compression).
- **Milestone 5.6:** Media REST API endpoints and metadata cataloging (MIME, EXIF, alt text, dimensions).

---

## Phase 6: Plugin Architecture & SDK
**Goal:** Safe, modular runtime extension without recompiling core CMS.

- **Milestone 6.1:** Plugin SDK specification and interface contracts.
- **Milestone 6.2:** Sandboxed plugin runtime (wazero WebAssembly and isolated RPC process).
- **Milestone 6.3:** Plugin lifecycle manager (Install -> Validate -> Configure -> Enable -> Disable -> Upgrade -> Uninstall).
- **Milestone 6.4:** Plugin extension points: Custom fields, admin widgets, lifecycle hooks, HTTP routes, jobs.
- **Milestone 6.5:** Reference plugins: Tamper-evident Audit Logs plugin and Rich Text Editor plugin.

---

## Phase 7: Admin Dashboard (React + TypeScript)
**Goal:** Modern, responsive administration interface built with React, Vite, Tailwind CSS v4, and shadcn/ui.

- **Milestone 7.1:** React admin setup with Vite, Tailwind CSS v4, TanStack Query, React Hook Form, and Zod.
- **Milestone 7.2:** Authentication & Session state management with token refresh.
- **Milestone 7.3:** Content Type Builder interface (drag-and-drop field configurations).
- **Milestone 7.4:** Dynamic Content Manager (data tables, filters, version history, draft/publish controls).
- **Milestone 7.5:** Media Library manager (folder navigation, preview, metadata editor, upload modal).
- **Milestone 7.6:** Theme and Plugin Management UI.
- **Milestone 7.7:** User, Role, Settings, and System Health dashboards.

---

## Phase 8: Advanced Capabilities (Localization, Search, Caching, Jobs, Webhooks, Multi-Tenancy)
**Goal:** Enterprise scale and multi-tenant operational features.

- **Milestone 8.1:** Content localization and translation management.
- **Milestone 8.2:** Search engine abstraction (in-memory, database full-text, external search engines).
- **Milestone 8.3:** Multi-tier caching with tag-based invalidation (HTTP, query, and template caching).
- **Milestone 8.4:** Background job system with retry policies, exponential backoff, and dead-letter queue.
- **Milestone 8.5:** Webhook dispatcher with SSRF protection, HMAC signatures, and delivery logs.
- **Milestone 8.6:** Multi-tenancy isolation (discriminator column vs dedicated database).
- **Milestone 8.7:** Optional GraphQL module.

---

## Phase 9: Ecommerce Subsystem
**Goal:** Modular ecommerce capabilities integrated seamlessly into the CMS.

- **Milestone 9.1:** Catalog management (products, variants, SKU, inventory movements).
- **Milestone 9.2:** Customer accounts and address books.
- **Milestone 9.3:** Shopping cart and checkout workflows.
- **Milestone 9.4:** Order lifecycle state machine, transactional inventory decrementing.
- **Milestone 9.5:** Pluggable payment and shipping provider interfaces.

---

## Phase 10: Production Hardening, Security Audit & Deployment
**Goal:** Verification against OWASP ASVS Level 2, automated security scanning, performance benchmarks, and deployment manifests.

- **Milestone 10.1:** Comprehensive security audit and penetration testing suite.
- **Milestone 10.2:** Static code analysis, vulnerability scanner integration (`govulncheck`, Trivy).
- **Milestone 10.3:** Performance benchmarks (API throughput, template rendering latency, concurrent load).
- **Milestone 10.4:** Kubernetes manifests, Helm charts, and production Docker configurations.
- **Milestone 10.5:** Complete developer documentation, tutorials, and release notes.
