# Field system audit and implementation plan

Audit date: 2026-11-11. This document records the baseline **before implementation**; completion and executed checks are tracked in `field-system-implementation.md`.

## Architecture and compatibility decisions

The existing Go service, repository interfaces, SQL adapters, event bus and REST routes remain the content engine. PostgreSQL/MySQL already store field configuration and draft/published/version documents as JSON. Components can therefore reuse `content_types` and `content_fields` with an additional `component` kind; component instances remain embedded in entry JSON. No existing values need conversion or deletion to add these capabilities. Component definitions cannot have independent content entries. Component UIDs use `category.name`; existing collection/single UIDs retain their current syntax.

Existing identifiers (`secret`, `number`, `decimal`, `relation_single`, `relation_multiple`, `repeatable_component`) are retained. Add `time` and `blocks`. UI metadata belongs in the existing configuration JSON, which must also be represented and validated in Go. Relations and media continue to persist stable IDs. Public API shapes remain unchanged; no implicit recursive population.

## Feature matrix

| Feature | Expected behavior | Current implementation | Missing functionality | Backend change | Frontend change | Migration risk | Priority |
|---|---|---|---|---|---|---|---|
| Field configuration | Typed, capability-driven settings | Small ValidationRules struct; builder hardcodes controls | Defaults, metadata, permission flags, correct capabilities | Validate settings independently and expose capabilities | Shared dynamic configuration dialog | Existing invalid configurations must remain readable | P0 |
| Text/email/URL/secret | Appropriate controls, constraints and safe handling | Basic text, email regex; secret lacks validation/hashing | Protocol safety, hashing, Unicode lengths | Typed validation, safe password handling | Correct inputs and errors | Legacy plaintext needs explicit repair | P0 |
| Numbers/boolean/temporal | Typed input, bounds, false is present | Number permits fractions; datetime-local mismatches RFC3339 | Time, bounds, timezone conversion | Integer validation, time and temporal bounds | Date/time/number controls | Reject invalid new writes without rewriting old data | P1 |
| JSON | Editable invalid intermediate text, valid persistence | Editor discards invalid edits | Buffered editor, schema constraints | Bounded JSON/schema validation | Format and validation UI | Keep stored strings until explicit repair | P1 |
| UID | Source picker, manual override, backend uniqueness | Source selector and generation exist; scan limited to 10,000 records | Concurrency-safe uniqueness, Unicode policy, auto/manual state | Persistence uniqueness and deterministic slug | Shared slug renderer | Conflicting existing values must block migration, not be dropped | P0 |
| Media | Browse/search/folders/upload/preview/order | Inline IDs and basic asset picker | Full existing-library integration, ID/category validation | Media lookup injection | Reusable modal picker | Existing URL strings require explicit mapping to IDs | P1 |
| Relations | Search, labels, pages, integrity and permissions | ID JSON, target lookup, picker | Reliable focus handling, strict item checks, deletion integrity | Check all IDs/type ownership/publication | Focus-contained picker and selected label hydration | Do not reinterpret strings or delete dangling values | P0 |
| Cardinality/inverses | Only advertise enforceable relationships | Single/multiple unidirectional only; relation table unused | Inverse relationship engine | Keep unsupported cardinalities unavailable | Describe supported semantics accurately | New inverse engine would need migration | P2 |
| Components | Reusable typed schemas, recursive editor | Only names and object shape checks | Definitions, cycles, recursion, safe deletion, nested checks | Component kind using existing schema storage | Component management in builder, recursive fields | Additive kind; older clients must not treat components as collections | P0 |
| Repeatable/dynamic zones | Ordered instances, typed discriminator, validation | Array shape checks and JSON textarea | Component lookup, allowed types, nested editor | Bounded recursive schema validation | Add/remove/duplicate/move/collapse | Legacy strings preserved and shown with repair warning | P0 |
| Rich text/blocks | Maintained editor, stable formats, sanitization | Rich text textarea; no blocks type | Framework, toolbar, structured documents | HTML sanitation and bounded document validation | Shared rich text and blocks renderer | Keep existing HTML; no automatic conversion | P1 |
| Builder | Type editing, grouped navigation, order, safe schema changes | Cards for collection/single; field CRUD; width UI | Component management, capability settings, metadata | Prevalidate schema, deletion guards | Extend existing builder and dialog | Current partial creates require atomic persistence | P1 |
| Layout | Order, grouping, responsive widths | field_order settings; width frontend only | Width persistence in Go, grouping, keyboard movement | Configuration metadata | Responsive shared field shell | Additive JSON settings | P1 |
| Listing | Server search/pages/sort/status, columns | First 50 entries with local search | Real query controls and column settings | Clamp filters; consistent memory/SQL queries | Listing controls and states | None | P1 |
| Draft/publish/history | Nested snapshots, optimistic locking | Existing lifecycle; shallow memory copies, ignored version errors | Deep isolation, atomic snapshots, restoration validation | Repository transactions and ownership checks | Correct raw mutation responses; publish updates | Preserve version numbering semantics | P0 |
| Permissions | Actual API protection and redaction | Schema writes check manage; entry writes only require login | Action permissions, route ownership, private/read-only field enforcement | Reuse current RBAC | Read-only/hidden controls and errors | New restrictions may reveal previously unsafe clients | P0 |
| Tests | Per-field, nested lifecycle, UI and integration | Go tests; no frontend test runner | New validators, UI interactions, end-to-end coverage | Go integration/regression tests | React test tooling | Test database availability must be reported | P0 |
| Adapters/tenancy | Preserve supported production behavior | SQL content repository; Mongo server does not mount content service; separate tenancy module | No complete tenant-scoped content engine | Do not claim tenant isolation or Mongo content parity | No unsupported controls | Existing architecture limitation | P0 documentation |

## Reference research

Reviewed official Strapi 5 documentation for behavior and terminology only:

- [Content-type Builder](https://docs.strapi.io/cms/features/content-type-builder): field settings, components, relations and schema workflow.
- [Content Manager](https://docs.strapi.io/cms/features/content-manager): editing, layout and selection workflows.
- [Models](https://docs.strapi.io/cms/backend-customization/models): attribute configuration, components and dynamic zones.
- [REST API](https://docs.strapi.io/cms/api/rest/): preserve explicit population boundaries rather than assuming every reference expands.

No Strapi source, assets or branding are imported.

## Verification phases

1. Audit/contracts (this document), baseline tests.
2. Field configuration and schema validation.
3. Typed scalar editors and validation.
4. UID/media/relation integration.
5. Reusable components and bounded recursive validation/rendering.
6. Dynamic zones and ordered instances.
7. Rich text and blocks, safe persistence.
8. Manager workflows and permissions.
9. Regression, migration verification and documentation.

Independent implementation streams may run concurrently, but integration must satisfy dependency checks. Never report a build or test as passed without executing it. Live database checks require an isolated test database; user content is not a test fixture.

## Current implementation checkpoint (2026-11-11)

The following work has been implemented in the existing engine; this is a verified checkpoint, not a claim that every item in the master prompt is complete.

| Area | Implemented in this checkpoint | Remaining / not yet verified |
|---|---|---|
| Field contracts | Added `time` and `blocks`; capability map; typed configuration/value validation; numeric/length/date/enum/JSON constraints; UTF-8/size/depth limits; allowed URL protocols; rich-text sanitization; blocks URL checks | Full JSON Schema draft support is intentionally not claimed; advanced unknown configuration keys still need lossless round-trip guarantees |
| Schema validation | Field key/config validation, UID sibling source checks, relation target checks, component reference/cycle/depth checks; component definitions reuse `content_types` with `kind=component` | No dedicated schema migration; migration/repair UI for legacy string-encoded advanced values remains manual and non-destructive |
| Persistence/lifecycle | Memory deep copies, atomic memory commits, optional atomic repository interfaces, SQL transaction commits for types/fields and entries/history, OCC and uniqueness checks | SQL implementation has not been exercised against a live isolated PostgreSQL instance in this checkpoint; MySQL transaction behavior also needs adapter integration tests |
| Content security | Per-action RBAC on mutations/history; entry-to-type ownership checks; component records cannot be opened as content; field read-only/edit-role checks; media ID/MIME validation hook; secret hashing and response redaction (including nested components) | Tenant-level isolation remains outside this content repository’s current architecture; media permission/tenant behavior needs integration validation |
| Builder | Existing builder now uses capability-driven add/edit dialog; field key generation, type-specific settings, UID source picker, content type/component selectors, enumeration rows, width, validation and schema-change acknowledgment; component definitions can be created and edited | Content-type rename/API UID editor, field drag ordering/grouping in Builder, full delete-impact preview and keyboard reorder remain outstanding |
| Content editor | Shared schema renderer for typed scalar, JSON, Tiptap rich text/blocks, UID, media, relations, nested components/repeatable components/dynamic zones; field widths; layout drag order; media search/upload/preview/removal/reorder; relation search/pagination/reorder | Media folder navigation and signed-URL refresh details, editor-level preview, complete column settings and bulk actions remain outstanding |
| Collection list | Server-side search, status filtering, sorting and pagination controls connected to existing query parameters | Filtering is currently limited to available server query semantics; configurable columns and bulk actions remain outstanding |
| Tests/docs | Added focused Go service/handler tests, frontend registry/renderer tests and this gap/implementation matrix | Full Go suite and UI production build are run at final verification; SQL integration and browser E2E are not claimed without execution |

### Storage and compatibility decision

No database migration is introduced by this checkpoint: field configuration and entry snapshots already use JSON storage, and reusable component definitions are additive `content_types` rows with `kind=component`. The SQL atomic repository uses transactions over the existing tables. Existing raw IDs and REST envelope conventions remain intact. New strict validation affects new writes and schema edits only; existing entry data is not rewritten automatically. Existing advanced values that are strings are shown as repair-needed by the editor and are preserved until explicitly repaired.

### Verified commands at this checkpoint

- Baseline before implementation: `go test ./internal/fields ./internal/content ./tests/e2e` (passed).
- After backend integration: `go test ./internal/content ./internal/fields ./cmd/server` (passed).
- Frontend: `npm.cmd test` (2 files, 4 tests passed); `npm.cmd run build` (passed, with the existing large-chunk warning).
- Final: `go test -count=1 ./...` passed across all Go packages, followed by `go build ./...` (passed with elevated access for the Windows module-cache write restriction).
- Final UI checks: `npm.cmd test` (2 files, 4 tests passed); `npm.cmd run build` (passed, with a large-chunk warning); `npm.cmd run lint` completed with React hook/ref warnings in the new editor and existing screens, no lint errors.
- No live PostgreSQL/MySQL integration run or browser E2E run is claimed. Existing unit/e2e package tests passed, but this implementation needs isolated-database and real browser validation before production rollout.
