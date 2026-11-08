# CMS Platform Architecture

## 1. Architectural Overview

The CMS is designed as a modular monolith in Go, balancing high performance, simplicity of deployment, and strong boundary isolation between modules. It supports two distinct operating modes:
- **Headless CMS Mode:** Exposes REST and GraphQL APIs for client applications (React, Next.js, mobile apps, microservices).
- **Traditional CMS Mode:** High-performance server-side rendered HTML using Go's standard `html/template` package, custom themes, assets, and partial caching.

### Core Architectural Principles
1. **Dependency Inversion:** High-level domain and application logic never depend on low-level database drivers, storage backends, or transport mechanisms. All external systems are accessed via interfaces defined in the domain and application layers.
2. **Layered Separation of Concerns:**
   - **Transport Layer:** HTTP handlers (REST/GraphQL), routing (Chi), request deserialization, input validation, context propagation, rate-limiting, and response serialization.
   - **Application Service Layer:** Orchestration of business operations, transactional boundaries, audit logging dispatch, lifecycle event triggers, and permission enforcement.
   - **Domain Layer:** Pure business entities, domain validations, state transitions (e.g. Draft -> Published), content field definitions, and domain errors.
   - **Repository / Infrastructure Layer:** Concrete database adapters (PostgreSQL, MySQL, MongoDB), object storage (S3, GCS, Azure, Local), caching (Redis/In-memory), search, and external integrations.
3. **No Unnecessary Abstractions:** Keep code clean, idiomatic Go. Avoid deep generic hierarchies or complex ORM magic that hides queries.
4. **Context-Aware Processing:** Every operation takes `context.Context` to handle cancellation, timeouts, tenant isolation, trace IDs, and security identities.
5. **Database Agnosticism with Explicit Capability Detection:** The CMS supports PostgreSQL, MySQL, and MongoDB. Features requiring specific database capabilities query the `DatabaseCapabilities` interface rather than guessing or breaking at runtime.

---

## 2. Directory Structure

```text
cms/
├── cmd/
│   ├── server/           # Main CMS server entry point
│   ├── migrate/          # Standalone database migration CLI tool
│   └── cms/              # CMS Developer CLI (scaffolding, doctor, management)
├── internal/
│   ├── core/             # Application lifecycle, service container, lifecycle events
│   ├── config/           # Configuration loader, schema validation, environment handling
│   ├── database/         # Database abstraction interfaces & capability matrix
│   │   ├── interfaces/   # Repository & connection contracts
│   │   ├── postgres/     # PostgreSQL pgx/database/sql implementation
│   │   ├── mysql/        # MySQL database/sql implementation
│   │   ├── mongodb/      # MongoDB official Go driver implementation
│   │   └── migrations/   # Migration runner and version tracking
│   ├── auth/             # Authentication, password hashing, sessions, tokens, MFA
│   ├── users/            # User account management & profile services
│   ├── permissions/      # RBAC, roles, fine-grained resource permissions
│   ├── content/          # Dynamic content storage, versioning, draft/publish lifecycle
│   ├── fields/           # Content type builder schema, field type registry & validators
│   ├── templates/        # Go html/template renderer, funcmap allowlist, layout loader
│   ├── themes/           # Theme manager, theme manifest validation, asset serving
│   ├── media/            # Media library, asset metadata, image processing pipeline
│   ├── storage/          # Storage provider interface & implementations (S3, GCS, Local)
│   ├── plugins/          # Plugin manager, isolation runtime (Wasm/RPC), hooks
│   ├── api/              # Dynamic REST router & OpenAPI generator
│   ├── webhooks/         # Webhook dispatcher, SSRF-safe HTTP client, retry worker
│   ├── cache/            # Multi-layer caching (in-memory, Redis, tags & invalidation)
│   ├── jobs/             # Background job runner, worker pool, scheduled tasks
│   ├── search/           # Search indexing and query abstraction
│   ├── localization/     # Content localization, language fallbacks, translations
│   ├── audit/            # Tamper-evident audit logging engine
│   ├── ecommerce/        # Optional ecommerce subsystem (products, carts, orders)
│   └── tenancy/          # Multi-tenant resolution, scoping, tenant isolation
├── pkg/
│   ├── plugin-sdk/       # Public SDK for building external plugins
│   ├── storage-sdk/      # Common storage provider interfaces & helper utilities
│   └── database-sdk/     # Database driver interfaces and migration abstractions
├── plugins/              # Built-in and user plugins
├── themes/               # Built-in and user themes
├── admin/                # React + Vite + Tailwind + shadcn/ui admin frontend
├── migrations/           # Database migration files (PostgreSQL, MySQL, MongoDB)
├── deployments/          # Docker, Kubernetes, Helm, and Compose configurations
├── docs/                 # System architecture, developer guides, and API specs
├── tests/                # Integration, E2E, and benchmark tests
├── Makefile              # Build, test, run, and migration automation
├── go.mod                # Go module definition
├── go.sum                # Go dependencies lock
└── README.md             # Project documentation
```

---

## 3. Request Flow & Layering

```mermaid
flowchart TD
    Client([HTTP / Browser Client]) --> Router[Chi HTTP Router & Middleware]
    Router --> Security[Security Headers, CORS, Rate Limiting, Tenant Context]
    Security --> AuthMiddleware[Auth & RBAC Middleware]
    
    subgraph Transport_Layer [Transport Layer]
        AuthMiddleware --> AdminHandler[Admin REST API Handlers]
        AuthMiddleware --> DynamicContentHandler[Dynamic Content API Handlers]
        AuthMiddleware --> PublicHTMLHandler[Public HTML Template Handler]
    end

    subgraph Service_Layer [Application Service Layer]
        AdminHandler --> AdminService[Admin & Schema Service]
        DynamicContentHandler --> ContentService[Content Engine Service]
        PublicHTMLHandler --> TemplateEngine[HTML/Template Renderer]
        TemplateEngine --> ContentService
    end

    subgraph Domain_Layer [Domain & Lifecycle Layer]
        ContentService --> LifecycleHooks[Lifecycle Event Bus (Before/After Hooks)]
        ContentService --> FieldValidator[Field Validation Engine]
        ContentService --> PermissionChecker[Resource Permission Verifier]
    end

    subgraph Infrastructure_Layer [Infrastructure & Persistence Layer]
        ContentService --> RepoInterface[Content Repository Interface]
        RepoInterface --> PostgresAdapter[PostgreSQL Adapter]
        RepoInterface --> MySQLAdapter[MySQL Adapter]
        RepoInterface --> MongoAdapter[MongoDB Adapter]
        
        ContentService --> CacheLayer[Cache Provider (Redis / Memory)]
        ContentService --> AuditLogger[Tamper-Evident Audit Log]
    end
```

---

## 4. Multi-Tenant Architecture

The CMS supports two tenancy models configurable at startup or per tenant:
1. **Shared Database / Tenant Scoping (Discriminator Column):**
   - Every content table and collection includes a indexed `tenant_id` column.
   - The `TenantContext` is injected into `context.Context` during early middleware evaluation via subdomain, header (`X-Tenant-ID`), or custom domain lookup.
   - All repository methods enforce `tenant_id = ?` matching from the context; missing tenant context results in immediate execution failure.
2. **Dedicated Database per Tenant (Enterprise Tier):**
   - Tenant database connections are pooled in a dynamic connection registry.
   - The repository resolves the active connection pool based on the tenant context.

---

## 5. Extensibility & Plugins

- External plugins run out-of-process or inside a sandboxed WebAssembly (Wasm) runtime (via `wazero`), communicating via strictly typed RPC.
- Plugins register custom fields, lifecycle hooks, API endpoints, and admin UI widgets.
- Core CMS operations never block indefinitely on plugin execution: strict timeouts, circuit breakers, and capability allowances isolate core stability.
