# Strapi 5 feature inventory and Go CMS parity audit

Audit/review date: 2026-11-11 (Asia/Calcutta). Comparison basis: the live official Strapi 5 documentation site, crawled/reviewed on this date. The documentation is continuously updated and does not expose a single pinned documentation build/version; pages reviewed explicitly describe behavior as recent as Strapi 5.57.0 (Content-type Builder) and 5.56.1 (Review Workflows). This is therefore a dated docs snapshot, not a claim that this repository was compared against a particular Strapi binary. The repository is MIT licensed and remains an independent Go implementation.

## Architecture and baseline

- Go module: `github.com/go-cms/cms`, Go 1.26.10; modular monolith with React/TypeScript admin.
- Existing modules include content/schema and draft/publish/history, fields, media (including HLS/folders), auth/RBAC/API tokens, localization, plugins, jobs, audit, search, themes/templates, storage, tenancy, database adapters, metrics, and OpenAPI.
- Existing databases/adapters include PostgreSQL, MySQL, and MongoDB packages. Content-manager service wiring and actual content persistence must be checked per adapter; adapter presence alone is not evidence of feature parity.
- Current field-system checkpoint and its detailed boundaries: [field-system-gap-analysis.md](field-system-gap-analysis.md). Existing field identifiers and API shapes are retained; the work does not replace the content engine.
- Baseline commands for this audit are recorded after execution below. No live database or browser E2E result is inferred from unit tests.

## Feature matrix

Status meanings: **Implemented** = code exists; **In progress** = partial implementation in this working tree; **Tested** = relevant automated tests passed; **Verified** = acceptance coverage across backend, UI, persistence, authorization and documentation. A package test alone does not make a platform feature Verified. Strapi plan labels are from official technical docs/pricing where stated; otherwise the entry says plan classification is not established by the cited feature page. `Community/free` describes the self-hosted CMS feature, not Strapi Cloud hosting.

| Feature | Strapi reference / classification | Current Go CMS status | Missing work | Priority |
|---|---|---|---|---|
| Content-Type Builder | [Builder](https://docs.strapi.io/cms/features/content-type-builder) — Community | In progress | Full schema staging/undo, groups, complete edit/settings, rename-safe migrations, stronger deletion impact preview and browser validation | P0 |
| Collection and Single Types | Builder — Community | Implemented | Confirm all edit and API compatibility semantics under live SQL | P0 |
| Field system and settings | [Fields](https://docs.strapi.io/cms/features/content-type-builder) — Community | In progress | Conditional visibility, localization options, private/search semantics, permission model, more capabilities and acceptance tests | P0 |
| Field width/layout | [Content Manager](https://docs.strapi.io/cms/features/content-manager) — Community | In progress | Persisted configurable editor layouts, groups/tabs and per-user view configuration | P1 |
| Custom fields | [Custom fields](https://docs.strapi.io/cms/features/content-type-builder) — plugin-provided | Not started | Stable Go/plugin field contract, admin renderer registration, backend validator registration and compatibility tests | P1 |
| Reusable components | Builder / [Content Manager](https://docs.strapi.io/cms/features/content-manager) — Community | In progress | Complete category/UID management, safe deletion/evolution UX, nested permissions/localization and full lifecycle verification | P0 |
| Repeatable components | Content Manager — Community | In progress | Component CRUD/ordering/duplication and exhaustive persistence/lifecycle coverage | P0 |
| Dynamic zones | Builder / Content Manager — Community | In progress | Allowed-component configuration/validation, stable IDs, reorder/duplicate UX, locale/history coverage | P0 |
| Relation fields | [Relations](https://docs.strapi.io/cms/features/content-type-builder) — Community | In progress | Only current one-way single/multiple semantics supported; inverse and other cardinalities need engine/migrations before exposure; enforce all permissions/integrity | P0 |
| Content Manager listing | [Content Manager](https://docs.strapi.io/cms/features/content-manager) — Community | In progress | Server-side filters, configurable columns, saved views, bulk operations, locale/status and permission edge coverage | P1 |
| Content Manager editing | Content Manager — Community | In progress | Remaining editor parity, layout settings, preview, accessibility and browser E2E | P0 |
| Draft & Publish | [Draft & Publish](https://docs.strapi.io/cms/features/draft-and-publish) — Community | Implemented | Verify nested fields/media/relations against real database adapters and concurrency | P0 |
| Content History | [Content History](https://docs.strapi.io/cms/features/content-history) — Enterprise | Implemented (basic version history exists) | Version comparison, robust restoration and full persisted nested-data/authorization coverage | P1 |
| REST Content API | [REST API](https://docs.strapi.io/cms/api/rest) — Community | Implemented | Strapi-style document APIs/query language/population/locale/status adapters, cost limits and complete OpenAPI coverage | P0 |
| Document identity/service | [Documents](https://docs.strapi.io/cms/api/document-service) — Community | Not started | Stable document identity spanning locale and draft/published variants; compatibility bridge to existing entry IDs | P1 |
| GraphQL API | [GraphQL](https://docs.strapi.io/cms/api/graphql) — plugin-provided (official GraphQL plugin) | Not started | Optional generated schema, queries/mutations, depth/cost limits, auth and tests | P2 |
| OpenAPI | [REST API](https://docs.strapi.io/cms/api/rest) — Community docs | Implemented (static spec/endpoint) | Generated/kept-in-sync spec and endpoint contract coverage | P1 |
| Media Library | [Media Library](https://docs.strapi.io/cms/features/media-library) — Community | In progress | Complete browser workflow, previews/metadata/selection integration, permission and signed URL coverage | P0 |
| Media folders | Media Library — Community | Implemented (backend support exists) | Fully integrated admin navigation and movement UX | P1 |
| Storage providers | [Upload providers](https://docs.strapi.io/cms/configurations/plugins) — providers/plugin ecosystem | Partial | Existing local/S3 abstractions; verify S3 compatibility and add/test GCS/Azure/MinIO providers if required | P1 |
| Image processing/variants | Media Library — provider/add-on ecosystem | Partial | Responsive variants, optimization configuration, limits and integration tests | P1 |
| HLS/video | No direct universal parity claim; provider/plugin capability | Implemented (HLS package support exists) | End-to-end playback authorization, provider and browser tests | P2 |
| Admin authentication | [Admin panel](https://docs.strapi.io/cms/features/admin-panel) — Community | Implemented | MFA/TOTP, recovery codes, invitation/reset flows and comprehensive session revocation/rate tests | P1 |
| RBAC | [RBAC](https://docs.strapi.io/cms/features/rbac) — Community | Implemented | Field/entry-level authorization and end-to-end policy matrix | P0 |
| API tokens | [API Tokens](https://docs.strapi.io/cms/features/api-tokens) — Community | Implemented | Custom scopes, expiration/rotation UX, broader verification | P1 |
| Users & Permissions (application users) | [Users & Permissions](https://docs.strapi.io/cms/features/users-permissions) — Community/plugin | Partial | Separate application-user auth/roles and user-facing auth flows | P1 |
| SSO / OIDC / SAML | [SSO](https://docs.strapi.io/cms/features/sso) — Enterprise/add-on | Not started | Standards-based identity provider integration, secure provisioning and role mapping | P2 |
| Internationalization | [Internationalization](https://docs.strapi.io/cms/features/internationalization) — Community | Partial | Locale variants integrated with content identity, localized fields/relations, UI and publication/history behavior | P0 |
| Scheduled publish/unpublish | [Releases](https://docs.strapi.io/cms/features/releases) — plan-dependent | Partial | Durable scheduler, timezone handling, retries and transactional/partial failure reporting | P1 |
| Releases | Releases — Enterprise | Not started | Multi-entry/locale release sets, scheduling, permission checks, execution audit and failure semantics | P2 |
| Review Workflows | [Review Workflows](https://docs.strapi.io/cms/features/review-workflows) — Enterprise | Not started | Workflow/stage model, assignment, transitions, publish gate, audit and UI | P2 |
| Preview / Live Preview | [Preview](https://docs.strapi.io/cms/features/preview) — plan-dependent (Live Preview paid tier per pricing) | Partial | Secure draft tokens, configurable origins/URLs and side-by-side live preview | P1 |
| Webhooks | [Webhooks](https://docs.strapi.io/cms/features/webhooks) — Community | Partial | Signed requests, event filters, retries, history UI, redaction and delivery tests | P1 |
| Lifecycle hooks / jobs | [Cron jobs](https://docs.strapi.io/cms/configurations/cron) — Community | Partial | Stable extension contracts, durable scheduling, monitoring and retry semantics | P1 |
| Email | [Email](https://docs.strapi.io/cms/features/email) — provider/plugin ecosystem | Partial | Provider SDK/config, templates, delivery/error handling and tests | P2 |
| Plugins / Marketplace | [Plugins](https://docs.strapi.io/cms/plugins) — Community/plugin ecosystem | Partial | Manifest lifecycle/dependencies, stable SDK/custom-field hooks, admin extensions and safe isolation | P1 |
| Search | [Search](https://docs.strapi.io/cms/features/search) — plugin/provider-specific | Partial | Query API integration, indexing lifecycle, permissions and production provider options | P1 |
| Audit logs | [Audit Logs](https://docs.strapi.io/cms/features/audit-logs) — Enterprise | Partial | Durable SQL repository, retention/export/filter UI, redaction and broader event coverage | P1 |
| Observability | [Monitoring](https://docs.strapi.io/cms/configurations/server) — deployment/provider dependent | Partial | Metrics are present; tracing, readiness, job monitoring and documented integrations need completion | P2 |
| Import/export/transfer | [Data management](https://docs.strapi.io/cms/data-management) — Community | Not started | Safe transfer formats, compatibility checks, relation/media mapping and transactional import/export | P1 |
| CLI / scaffolding | [CLI](https://docs.strapi.io/cms/cli) — Community | Not started | `go-cms` project create/develop/build/doctor/generate/plugin commands and generated-project smoke tests | P1 |
| Go themes/templates | No direct exact analog; architecture-specific | Implemented | Production cache/preview/security acceptance and developer docs | P2 |
| Database support | [Database configuration](https://docs.strapi.io/cms/configurations/database) — Community | Partial | Repository has PostgreSQL/MySQL/Mongo adapters; prove content service parity for each before claiming support | P0 |
| Deployment/Docker | [Deployment](https://docs.strapi.io/cms/deployment) — Community/docs | Partial | Clean install, Compose, upgrades, backups/restores and release artifacts | P1 |
| Security | [Security](https://docs.strapi.io/cms/configurations/security) — cross-cutting | Partial | ASVS-informed controls/evidence, field/tenant checks, upload and auth hardening; no certification claim | P0 |

## Key architectural decisions

1. Keep the Go modular monolith and current content service/repository contracts; add adapters rather than a second content engine.
2. Keep current field identifiers and JSON persistence compatibility. `number`/`decimal` represent integer/float choices through configuration/validation where the existing storage allows; distinct aliases should not be introduced until migration and serialization behavior is defined.
3. Relations exposed today are explicitly one-way `relation_single` (many-to-one) and `relation_multiple` (many-to-many). Do not imply inverse relation synchronization until implemented and tested.
4. Components/dynamic zones remain schema-driven nested values. Validate recursive depth and allowed component definitions server-side; never treat frontend hiding as authorization.
5. Schema and content mutations must preserve existing draft, publish, history, permissions and IDs; schema type changes/deletions require explicit impact planning.
6. GraphQL, SSO, workflows, releases and CLI are separate later milestones, not hidden claims of this field-system work.

## Milestone tracking / current continuation point

| Milestone | State | Evidence / remaining |
|---|---|---|
| M0 Repository and documentation audit | Tested | Go full-suite baseline and admin tests passed; official feature pages and official pricing matrix reviewed 2026-11-11. Live database and browser E2E remain outside this audit run. |
| M1 Complete field system | In progress | See [field-system-gap-analysis.md](field-system-gap-analysis.md); code/tests exist for scalar, structured, relation, media, UID, component, and dynamic-zone slices. Full DB/browser coverage is outstanding. |
| M2 Content modeling | In progress | Existing builder extended; component lifecycle and safe schema evolution remain incomplete. |
| M3 Content Manager | In progress | Listing/editor extended; saved layouts, bulk actions, full filters and browser acceptance remain. |
| M4 Content APIs | In progress | Existing REST/OpenAPI retained; document identity, full query semantics/population and optional GraphQL remain. |
| M5 Media and localization | In progress | Existing modules exist; integration/parity gaps above remain. |
| M6 Security and administration | In progress | Auth/RBAC/tokens exist; MFA/SSO and field-level coverage remain. |
| M7 Editorial tools | Not started / partial foundations | Existing draft/publish/history/jobs/audit modules; workflows/releases/scheduling remain. |
| M8 Extensibility | In progress | Plugin manager exists; stable SDK surface/custom fields/isolation remain. |
| M9 Distribution | Not started | CLI scaffolding, artifact/release automation and clean-project verification remain. |
| M10 Production hardening | In progress | Unit tests exist; live database, browser, load, backup/restore and independent security review remain. |

## Official sources reviewed

- [Content-type Builder](https://docs.strapi.io/cms/features/content-type-builder) — field configuration, relations, components, dynamic zones, conditional fields and schema behavior; current page references 5.57.0 field renaming.
- [Content Manager](https://docs.strapi.io/cms/features/content-manager) — editing, relations, components, dynamic zones and ordering.
- [Media Library](https://docs.strapi.io/cms/features/media-library) — current UI since 5.54.0, providers, folders and API.
- [REST API](https://docs.strapi.io/cms/api/rest/) and [GraphQL API](https://docs.strapi.io/cms/api/graphql) — API query/mutation behavior and plugin status.
- [Internationalization](https://docs.strapi.io/cms/features/internationalization) — locale behavior; feature docs include current-version notes such as relation preservation from 5.42.1.
- [Review Workflows](https://docs.strapi.io/cms/features/review-workflows) and [Releases](https://docs.strapi.io/cms/features/releases) — Enterprise classifications and current behavior.
- [Official pricing/plan matrix](https://strapi.io/pricing) — Community versus Growth/Enterprise feature labels. Plan names/entitlements can change; re-check before release planning.

## Verification record

Audit baseline and post-change checks executed on 2026-11-11:

- `go test -count=1 ./...` — passed across all Go packages.
- `npm.cmd test` in `admin/` — passed: 3 test files, 6 tests (includes the relation-picker auto-suggestion regression test).
- `npm.cmd run build` in `admin/` — passed; Vite reports the existing ~1.4 MB minified JS chunk warning.
- `git diff --check` — passed (Git emitted line-ending/config ignore access warnings, not whitespace errors).
- No live PostgreSQL/MySQL integration, browser E2E, GraphQL, or generated-project test is claimed.

No feature is marked Verified based only on static inspection, and no database/browser test is claimed without executing it against an isolated fixture.
