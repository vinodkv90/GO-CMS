# Extensible Go-Based CMS Platform

A production-grade, extensible Content Management System (CMS) built in Go.

Combines the dynamic content modelling and headless API flexibility of modern headless platforms (Strapi) with traditional server-rendered HTML websites using Go standard library `html/template` (WordPress style), a plugin ecosystem, media library, and enterprise multi-database support (PostgreSQL, MySQL, MongoDB).

---

## Key Features

- **Dual Operating Modes:**
  - **Headless Mode:** Dynamic REST (and GraphQL) content APIs for React, Next.js, mobile apps.
  - **Traditional Mode:** Native server-side rendered HTML using strictly Go standard `html/template`.
- **Database Abstraction with Capability Detection:** Native adapters for PostgreSQL, MySQL, and MongoDB. Explicit capability matrix prevents unsupported operations at runtime.
- **Enterprise Security:** Built to OWASP ASVS Level 2 standards. Hardened HTTP headers, HMAC-SHA256 signed URLs, SSRF protection, strict input validation, contextual auto-escaping.
- **Production Architecture:** Modular monolith with dependency inversion, structured `log/slog` logging, graceful shutdown, health/readiness endpoints, and Docker deployment.

---

## Architecture & Documentation

- [System Architecture](docs/ARCHITECTURE.md)
- [Implementation Roadmap](docs/ROADMAP.md)
- [Database Capabilities Matrix](docs/DATABASE-CAPABILITIES.md)
- [Security Architecture](docs/SECURITY.md)
- [Plugin SDK Specification](docs/PLUGIN-SDK.md)
- [Template Engine & Theme Guide](docs/TEMPLATE-ENGINE.md)
- [Live Implementation Status](docs/IMPLEMENTATION-STATUS.md)

---

## Quick Start

### 1. Requirements
- Go 1.24+
- Docker & Docker Compose (optional for local database services)

### 2. Configuration
Copy the configuration template:
```bash
cp config.example.yaml config.yaml
```

### 3. Start Database Services (Docker)
```bash
docker compose up -d postgres redis
```

### 4. Build and Run Server
```bash
go run ./cmd/server
```
The server starts at `http://localhost:8080`.

### 5. Check Health & Info
```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
curl http://localhost:8080/info
```

### 6. Run Test Suite
```bash
go test -v ./...
```

