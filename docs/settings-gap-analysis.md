# Go CMS Settings & Administration — Gap Analysis & Implementation Architecture

Audit date: 2026-11-12. Comparison baseline: official [Strapi 5 documentation](https://docs.strapi.io/) (`features/admin-panel`, `features/rbac`, `features/users-permissions`, `features/api-tokens`, `features/admin-tokens`, `features/data-management`, `features/media-library`, `features/email`, `features/internationalization`, `backend-customization/webhooks`, `features/sso`, `features/audit-logs`). This document records the audited repository baseline prior to Settings implementation and establishes the database, security, API, and frontend design for Milestones 1–7.

---

## 1. Current Features

### 1.1 Backend Modules Present
- **Configuration Loader (`internal/config/config.go`)**: Loads YAML + `CMS_*` environment variables for `Server`, `Database` (`postgres` / `mysql` / `mongodb`), `Logging`, `Auth` (`jwt_secret`, `session_secret`, `token_expiration`), `Media` (`provider`, `upload_dir`, `signed_urls`), `Themes`, `Plugins`, `Cache` (`memory` / `redis`), and `Jobs`. Read-only at runtime.
- **Admin Authentication & RBAC (`internal/auth/*`)**:
  - Argon2id password hashing (`password.go`, hardcoded strength check: `>= 8` chars, upper, lower, digit).
  - HS256 JWT tokens (`jwt.go`) delivered via `cms_token` HttpOnly cookie + JSON body, plus `cms_csrf` cookie + `CSRFMiddleware`.
  - Role-based access control (`rbac.go`) supporting `super_admin` bypass, `action`/`resource` permissions, and prefix wildcards (`resource:*`).
  - Default seeded roles: `super_admin`, `editor`, `author`, `viewer` (`auth/service.go`).
  - Personal Access Tokens (`cms_pat_<hex>`, SHA-256 hashed in `cms_api_tokens`, scoped via `action:resource` permissions + optional `expires_at` + `last_used_at`).
- **API Infrastructure (`internal/api/*`)**:
  - `chi` router with `RequestID`, `TrustedRealIPMiddleware`, `StructuredLoggerMiddleware`, `Recoverer`, security headers/CSP, dynamic body limits, per-IP token-bucket `IPRateLimiter` (`50 req/s`, burst `100` hardcoded), Prometheus HTTP metrics, timeout, CORS (`go-chi/cors`), tenancy header/query middleware, auth, and CSRF.
- **Media & Storage (`internal/media/*`, `internal/storage/*`)**:
  - Local filesystem storage (`storage/local`), HMAC-SHA256 signed URL verifier (`storage/signed.go`), folder tree CRUD, MIME magic-byte sniffing, thumbnail generation, HLS `.zip` upload validation & playlist URL rewriting.
- **Platform Subsystems**:
  - **Audit (`internal/audit`)**: Service + `HTTPHandler`, wired in `api/server.go:132` with `MemoryRepository` (`nil` DB repo).
  - **Plugins (`internal/plugins`)**: `Manager` + `HostAdapter`, native plugins `audit-log` (writes `cms_audit_logs`) and `seo-analyzer`, custom-field registry hook, enable/disable + settings JSON patch in memory.
  - **Localization (`internal/localization`)**: `Service` + `HTTPHandler` for `Locale` CRUD + default locale (in-memory, seeded `"en"`).
  - **Background Jobs (`internal/jobs`)**: `Queue` + `WorkerPool` + `HTTPHandler` (`jobs`, `jobs/stats`, `jobs/{id}/retry`) in memory.
  - **Cache (`internal/cache`)**: `MemoryCache` and `RedisCache` with tag-based `DeleteByTag` subscribed to content lifecycle events.
  - **Tenancy (`internal/tenancy`)**: In-memory tenant CRUD + `X-Tenant-ID` / `?tenant=` middleware.
  - **Themes & Templates (`internal/themes`, `internal/templates`)**: Directory scanner + `html/template` renderer; `themes/default/theme.json` defines `accentColor`, `showAuthor`, `postsPerPage`.
  - **Database Migrations (`internal/database/migrations`)**: Versioned `Runner` tracking `cms_schema_migrations` across `postgres`, `mysql`, and `mongodb`.

### 1.2 Admin Frontend Present (`admin/src/*`)
- **Shell**: Dark slate sidebar (`Sidebar.tsx`), white topbar (`Topbar.tsx`), responsive content layout (`Layout.tsx`), tab-state session persistence in `App.tsx`.
- **Views**: `DashboardView`, `ContentTypesView` (Strapi 5–inspired Content-Type Builder), `ContentManagerView` (Strapi 5–inspired entry/component/dynamic-zone editor), `MediaLibraryView`, `PluginsView`, `ThemesView`, `UsersView` (read-only table calling `GET /api/v1/users`), `LoginView`.
- **Shared UI Library (`admin/src/components/ui/*`)**: `Button`, `Badge`, `Card`, `Input`, `Modal`, `ConfirmDialog`, `Switch`, `ToastProvider` / `useToast` built on Tailwind v4 + `lucide-react`.

---

## 2. Missing Settings & Parity Gaps (vs Prompt & Strapi 5 Reference)

| Domain | Strapi 5 / Target Capability | Current Status in Go CMS | Gap Summary |
|---|---|---|---|
| **Settings Shell & Nav** | Grouped `Settings` navigation (Global Settings, Administration Panel, Users & Permissions, API Configuration, System), breadcrumbs, unsaved changes guards, save/cancel bars | Absent (`Sidebar.tsx` has no Settings section) | Need Settings workspace hub, sub-nav, RBAC link gating, reusable dirty-guard bar |
| **General Settings** | App name, description, public URL, admin base path info, default timezone/locale, branding (logo/favicon upload), date/time display format | Hardcoded `"Go CMS"` / `"Modern Extensible CMS Built in Go"` in `templates/renderer.go:583-589` | `cms_settings` table unused; need typed settings storage + live template/branding wiring |
| **Overview & System Info** | Overview + read-only runtime info (CMS/Go version, env, DB driver & migration status, storage, cache, jobs, enabled plugins, uptime, health) | `/info`, `/readyz`, `/healthz` exist at root (not `/api/v1`); omitted from admin UI & unauthenticated | Need authenticated `/api/v1/settings/system` endpoint exposing sanitized runtime telemetry (zero secrets) + System pages (`Environment`, `Database`, `Cache`, `Background Jobs`, `Storage Providers`) |
| **User Profile & Prefs** | Per-admin profile edit, change password with live policy hints, UI theme/locale preferences | Only read-only Topbar user name/role | Need `PATCH /api/v1/auth/me`, self password change endpoint, `cms_user_preferences`, interactive account menu |
| **Admin Users & Invites** | Create/edit/search/filter admins, assign roles, activate/deactivate, safe deletion (`last Super Admin` protection), secure single-use hashed expiring email invitations + resend/revoke | `UsersView` is read-only; backend `POST /users` ignores roles/status; no invitations table/workflow; no last-Super-Admin check | Need full Admin Users UI + search/status filter + `cms_admin_invitations` + invite acceptance flow + last Super Admin guard |
| **Admin Roles & RBAC** | Role CRUD + duplicate, safe delete, permission matrix across Modules/Content Types/Fields/Media/Settings/Plugins, full backend enforcement | Only `GET /api/v1/roles`; no create/update/delete/duplicate; handler permissions missing from seed | Need role CRUD endpoints, granular matrix UI, dynamic content-type/module permission enforcement, seeded handler permissions |
| **API Tokens & Transfer Tokens** | API Tokens UI + metadata + Read-only/Full-access/Custom types, expiration presets, one-time reveal, rotation/regenerate, revocation, kind separation | PAT API exists (`POST/GET/DELETE /api/v1/api-tokens`) with **no UI**, no token types/descriptions/rotate, and privilege-escalation bugs | Fix PAT security checks, add token types/rotation/prefix display/Admin enforcement + Transfer Tokens support gated by config salt or policy |
| **REST API, CORS & Rate Limits** | Pagination defaults/max, population depth, query complexity, OpenAPI/docs toggle, body size visibility, CORS origins/methods/headers/credentials validation, trusted proxies & rate limit config | Hardcoded in `api/server.go:75-105` & `content/handler.go`; unsafe default `*` + `AllowCredentials: true` | Add runtime API & CORS/Security policy persistence (`cms_settings`), dynamic middleware evaluation, reject wildcard+credentials CORS |
| **Users & Permissions (App Users)** | Strict separation from Admin Users: `Public`, `Authenticated`, custom app roles, endpoint/content permissions, app-user CRUD/block/confirm/reset, auth toggles (sign-up, email confirmation, default role, session/password/lockout policies), OAuth/OIDC providers | **Not implemented** (`POST /auth/register` mixes into `cms_users`; no app roles or OAuth/OIDC) | Add `cms_app_roles`, `cms_app_users`, `cms_auth_providers`, dedicated `/api/v1/app-auth/*` and `/api/v1/admin/users-permissions/*` routes + admin UI |
| **Media Library & Storage** | Max upload size, allowed categories, responsive breakpoints, image optimization, filename normalization, default folder, public vs signed URL default + TTL, S3/GCS/Azure/MinIO provider status & safe switching warning | Limits/thumbnails hardcoded; `Media.Provider` ignored (`cmd/server/main.go:103` hardcodes `local`); `s3.go` stubbed; thumbnail MIME bug | Persist media policy in `cms_settings`, enforce live in `media.Handler`/`media.Service`, fix thumbnail format/responsive generation, implement S3/MinIO-compatible provider & honest GCS/Azure config status |
| **Email Config & Templates** | Provider selection (SMTP / disabled/sendmail/log), sender name/address, reply-to, masked secret credentials, test connection & test email, 5 sanitized HTML/text templates with preview & variable docs | Only unused constant `jobs.TypeSendEmail` exists | Build `internal/email` (`net/smtp` + test connection + template renderer with `html/template` escaping), encrypted secret storage, UI forms & preview |
| **Security & Authentication Policy** | Password policy (length/upper/lower/digit/special), account lockout + failed attempt counter, login rate limiting, MFA/TOTP, SSO/OIDC auto-provisioning policy | Fixed regex password check; zero lockout, zero MFA, zero SSO, hardcoded login limits | Enforce configurable password policy & lockout on login, add TOTP MFA + recovery codes + OIDC provider config |
| **Webhooks** | CRUD, enable/disable, per-event & per-content-type selection, custom headers, HMAC payload signing, SSRF-safe delivery client, retries, delivery logs & test trigger | Only unused constant `jobs.TypeWebhookDispatch` exists | Build `internal/webhooks` (tables, SSRF-guarded HTTP dialer from `SECURITY.md`, HMAC-SHA256 signing, `EventBus` subscriber, worker retries, delivery history + UI) |
| **Plugins Settings** | Schema-driven config forms, persistent enable/disable/settings across restarts, accurate `/api/v1/plugins/ext/*` auth & permission enforcement | In-memory only; raw JSON textarea; `/ext/*` unauthenticated; missing schemas on built-in plugins | Persist `cms_plugins_state`, add `SettingsSchema` to plugins, render structured schema UI in Settings > Plugins, fix `/ext/*` auth & cleanup |
| **Internationalization** | Persistent locales CRUD, default locale, delete-safety guard against orphaned content, UI | In-memory `localization.Service` only; disconnected from settings | Persist `cms_locales`, block unsafe default/in-use deletions or warn cleanly, build Internationalization settings UI |
| **Audit Logs** | Persistent unified log store (`cms_audit_logs`), filters (actor, action, resource, date), paginated table, detail drawer with secret redaction, CSV export | Disconnected: `server.go` uses `MemoryRepository` (`nil`), plugin writes `cms_audit_logs` directly with hardcoded `"system"`/`"127.0.0.1"` | Wire unified `audit.SQLRepository`, record all auth/RBAC/token/settings/media/webhook/plugin/user changes, redact secrets, build Audit Logs UI |

---

## 3. Existing Backend Endpoints Inventory

### Root / Operational Endpoints (`internal/api/server.go`)
- `GET /healthz`, `GET /readyz`, `GET /info`, `GET /docs`, `ALL /metrics` (currently unauthenticated; `/info` and `/metrics` expose operational details).
- `GET /uploads/*` (Media file server with signed-URL verification when enabled).
- `GET /admin/*`, web/theme routes (`internal/templates/renderer.go`).

### `/api/v1` Endpoints
- **Auth & Identity (`internal/auth/handler.go`)**:
  - `POST /api/v1/auth/login`, `POST /api/v1/auth/register`, `POST /api/v1/auth/logout`, `GET /api/v1/auth/me`
  - `GET/POST /api/v1/users`, `GET/PATCH/DELETE /api/v1/users/{id}`
  - `GET /api/v1/roles`
  - `POST/GET /api/v1/api-tokens`, `DELETE /api/v1/api-tokens/{id}`
- **Content & Schema (`internal/content/handler.go`)**:
  - `GET /api/v1/content-types`, `/field-capabilities`, `/custom-fields`, `/{uid}`
  - `POST/PUT/DELETE /api/v1/content-types`, `/{uid}`, `/{uid}/fields`, `/{uid}/fields/{fieldKey}`
  - `GET/POST/PUT/DELETE /api/v1/content/{uid}...` + `/publish`, `/unpublish`, `/versions`, `/restore/{version}`
- **Media (`internal/media/handler.go`)**:
  - `GET/POST/PUT/PATCH/DELETE /api/v1/media...` (`/tree`, `/folders`, `/upload`, `/hls`, `/{id}/signed-url`, `/move`)
- **Plugins (`internal/plugins/handler.go`)**:
  - `ALL /api/v1/plugins/ext/{pluginID}/*` (currently public/unauthenticated)
  - `GET /api/v1/plugins`, `GET /api/v1/plugins/{id}`, `POST /enable`, `POST /disable`, `PATCH /settings`
- **System Modules**:
  - `GET/POST /api/v1/jobs`, `/jobs/stats`, `/jobs/{id}`, `/jobs/{id}/retry` (`internal/jobs/handler.go`)
  - `GET/POST/PUT/DELETE /api/v1/locales...` (`internal/localization/handler.go`)
  - `GET /api/v1/audit-logs` (`internal/audit/handler.go`)
  - `GET/POST/PATCH/DELETE /api/v1/tenants...` (`internal/tenancy/handler.go`)
  - `GET /api/v1/themes`, `GET /api/v1/search`, `GET /api/v1/ping`, `GET /api/v1/openapi.yaml`

---

## 4. Required Frontend Components & Views

### 4.1 Reusable UI Primitives Needed (`admin/src/components/ui/` & `admin/src/components/settings/`)
- `Select`, `Textarea` (with optional `monospace`), `Alert`/`Callout`, `DataTable` + `Pagination`, `Drawer`, `CopyableSecret` (one-time copy + masked display), `SecretInput` (eye reveal toggle / preserved-secret `"leave blank to keep existing"` mode), `StatusIndicator`, `EmptyState`, `Skeleton`, `cn()` utility (`clsx` + `tailwind-merge`).
- Upgrade `Modal.tsx` with `role="dialog"`, `aria-modal="true"`, focus trap, and scroll lock; expand `Badge.tsx` variants (`error`, `neutral`).
- Reusable Settings domain components:
  - `SettingsLayout` (grouped left Settings nav matching Strapi 5 sections, breadcrumbs, page header, Save/Cancel dirty action bar, navigation unsaved-changes guard)
  - `PermissionMatrix` (module/content-type/field action grid + Select-all toggles + read-only Super Admin mode)
  - `UserTable` & `InviteAdminModal` (including one-time invite link banner & copy action)
  - `RoleEditor` (details + permission matrix + assigned users count)
  - `ApiTokenEditor` & `OneTimeTokenBanner`
  - `SchemaSettingsRenderer` (renders plugin `SettingsSchema` typed controls cleanly)
  - `WebhookEditor` & `WebhookDeliveryModal`
  - `MediaSettingsForm`, `EmailSettingsForm`, `EmailTemplateEditor`

### 4.2 Views & Navigation Integration
- Top-level Settings entry in `Sidebar.tsx` (permission-gated) + topbar Account menu (`Profile` + `Sign Out`) + `hasPermission(user, action, resource)` RBAC guard utility.
- Settings screens across `Global Settings` (`Overview`, `General`, `Internationalization`, `Media Library`, `Email Configuration`, `Webhooks`, `Plugins`), `Administration Panel` (`Roles`, `Users`, `Authentication & MFA`, `Single Sign-On`, `Security`, `Audit Logs`), `Users & Permissions` (`Roles`, `Users`, `Authentication Providers`, `Email Templates`, `Advanced Settings`), `API Configuration` (`API Tokens`, `Transfer Tokens`, `REST API Settings`, `CORS Settings`, `Rate Limiting`), and `System` (`Environment`, `Database`, `Storage Providers`, `Cache`, `Background Jobs`, `System Information`).

---

## 5. Required Database Changes (`internal/database/migrations`)

Reuse existing tables (`cms_roles`, `cms_permissions`, `cms_role_permissions`, `cms_users`, `cms_user_roles`, `cms_api_tokens`, `cms_settings`, `cms_audit_logs`, `cms_media`, `cms_media_folders`) and add backward-compatible migrations across `postgres`, `mysql`, and `mongodb`:

1. **Extend `cms_users`**: `last_login_at`, `failed_login_attempts`, `locked_until`, `password_changed_at`, `mfa_enabled`, `mfa_secret_enc`, `mfa_backup_codes_hash`, `sso_provider`, `sso_subject`.
2. **Extend `cms_api_tokens`**: `kind` (`'api' | 'admin' | 'transfer'`), `description`, `token_type` (`'read-only' | 'full-access' | 'custom' | 'push' | 'pull'`), `token_prefix`, `revoked_at`.
3. **Extend `cms_audit_logs`**: align columns (`actor_type`, `target_id`, `user_agent`, `result`, plus indexes on `(created_at DESC)`, `action`, `resource_type`, `user_id`) so `audit.Service` and `plugins/audit-log` share one durable SQL table.
4. **New Administration & Settings Tables**:
   - `cms_user_preferences` (`user_id` PK FK -> `cms_users`, `theme`, `locale`, `preferences`, `updated_at`)
   - `cms_admin_invitations` (`id`, `email`, `first_name`, `last_name`, `role_ids`, `token_hash` UNIQUE, `invited_by` FK -> `cms_users` ON DELETE SET NULL, `status`, `expires_at`, `accepted_at`, `created_at`)
   - `cms_app_roles` & `cms_app_users` (separated end-user API roles/accounts with `role_id` FK, `confirmed`, `blocked`, confirmation/reset hashed tokens)
   - `cms_oauth_providers` (`provider` PK, `display_name`, `enabled`, `client_id`, `client_secret_enc`, `auth_url`, `token_url`, `userinfo_url`, `issuer_url`, `scopes`, `target` [`'admin' | 'app'`], `updated_at`)
   - `cms_email_templates` (`id`, `template_key` UNIQUE, `name`, `shipper_name`, `shipper_email`, `response_email`, `subject`, `html_body`, `text_body`, `updated_at`)
   - `cms_webhooks` & `cms_webhook_deliveries` (`webhook_id` FK ON DELETE CASCADE)
   - `cms_locales` (`code` PK, `name`, `direction`, `is_default`, `created_at`)
   - `cms_plugins_state` (`plugin_id` PK, `enabled`, `settings`, `updated_at`)

Rollback SQL/Mongo steps must be documented and non-destructive to existing entries, media, and admin accounts.

---

## 6. Security Risks & Remediations

| Severity | Finding in Current Code | Required Remediation |
|---|---|---|
| **Critical** | **`POST /api/v1/api-tokens` Privilege Escalation**: Any authenticated user can create a PAT with `["*:*"]`; `RequirePermission` trusts token scope without checking owner's RBAC permissions, and inline `HasPermission(user, ...)` checks ignore PAT restrictions. | Restrict token creation/management by RBAC & intersection with creator's own permissions; enforce **both** token permissions AND owner RBAC permissions on every request; honor token scope in `GetAPITokenFromContext` checks. |
| **Critical** | **CSRF Bypass via Arbitrary `Authorization` Header** (`auth/middleware.go:218-240`): Any non-`Bearer` `Authorization` value skips `CSRFMiddleware` while `extractToken` still authenticates via `cms_token` cookie. | Skip CSRF **only** when authentication actually succeeded via `Authorization: Bearer ...` or `X-API-Key` header (never when falling back to cookies). |
| **High** | **Insecure Default CORS (`["*"]` + `AllowCredentials: true`)** (`config.go:117`, `server.go:94-105`). | Disallow wildcard `*` together with `AllowCredentials: true`; enforce validation in both runtime CORS Settings and startup `Config.Validate()`. |
| **High** | **Missing Brute-Force / Account Lockout & Permissive Global Auth Rate Limit**: `SECURITY.md` documents lockout & rate limits that do not exist in code. | Enforce configurable failed-login counter + lockout window (`cms_users`), auth-endpoint rate limiter, and `settings` policy validation. |
| **High** | **Open Public Admin Self-Registration (`POST /api/v1/auth/register`)**: Allows anyone to register rows into `cms_users`. | Separate end-user signup into `/api/v1/app-auth/register` (`cms_app_users`, controlled by Users & Permissions `enable_signups`); keep admin creation invite/admin-only. |
| **High** | **Missing Last-Super-Admin Protection & Role/User Validation**: Admins could deactivate/delete the sole `super_admin` or mutate invalid statuses. | Block deletion, deactivation, or `super_admin` role removal of the last active Super Admin inside transactional user/role updates; prevent self-deletion. |
| **High** | **Unauthenticated Operational & Plugin Subroutes**: `/info`, `/metrics`, `/api/v1/plugins/ext/*` expose system/plugin endpoints without auth. | Protect `/info` behind authenticated admin/settings read; enforce `RouteDef.Permission` / auth in plugin dispatcher; allow disabling `/metrics` or gating behind token/internal policy. |
| **Medium** | **JWT Validation & Cookie Gaps**: `ValidateAccessToken` skips `iss`/`aud`; `ClearAuthCookie` omits `Secure`; `isProductionSecret` passes `config.example.yaml` placeholder strings. | Enforce `Issuer`/`Audience` claims, match `Secure` flag in cookie clearing, tighten secret placeholder detection, derive encrypted storage key cleanly for stored SMTP/OAuth/Webhook secrets, never return raw secrets to frontend. |
| **Medium** | **Unbounded Unsorted SSRF Risk for Upcoming Webhooks**: Outbound HTTP delivery can hit internal/metadata IPs if naive `http.Client` is used. | Use custom `net.Dialer` validating resolved IPs against RFC1918/loopback/link-local/CGNAT before socket connection (`SECURITY.md` §2.4), disallow redirects to private IPs, timeout bounds. |
| **Low/Med** | **Silent Disconnected Audit Trail**: Core security/admin actions never write to `cms_audit_logs`. | Wire durable `audit.SQLRepository` across Auth, Users, Roles, Tokens, Settings, Media, Plugins, Locales, Webhooks, with secret redaction. |

---

## 7. Implementation Priorities (Milestones 1–7)

1. **Pass 1 — Foundation, Administration & Security Hardening (Milestones 1–2)**:
   - **Milestone 1 (Settings Foundation)**: Core migration (`202611120001`), typed `internal/settings` service + cache + `cms_settings` + `cms_user_preferences`, General Settings & `/api/v1/settings/system`, Profile + Change Password, grouped Strapi-inspired Settings layout/sidebar/breadcrumbs/dirty guards, General Settings UI, Overview, System Information pages (`Environment`, `Database`, `Cache`, `Background Jobs`, `Storage Providers`).
   - **Milestone 2 (Administration, RBAC & Auth Hardening)**: Seed missing handler RBAC permissions; Admin Role CRUD + Permission Matrix UI + last-Super-Admin guard; Admin Users CRUD/status/search/filter + hashed single-use expiring `cms_admin_invitations` + invite acceptance flow; fix CSRF non-Bearer bypass & JWT `iss`/`aud` check.
2. **Pass 2 — API Management, Media/Email & Security/Users-Permissions (Milestones 3–5)**:
   - **Milestone 3 (API Management)**: Fix PAT privilege escalation & scope enforcement; add API Token types (`read-only`/`full-access`/`custom`), descriptions, rotation/regeneration, revocation, Admin & Transfer Tokens tabs; REST API settings (`pagination`, `max_depth`, `complexity`, `body_limit`, `docs_enabled`); live configurable Rate Limiting + CORS settings (blocking wildcard + credentials).
   - **Milestone 4 (Media & Email)**: Persist & enforce Media Library settings (`max_upload_mb`, allowed types, image optimization/auto-orient, responsive thumbnails, filename normalization, access mode, signed URL TTL); implement working S3/MinIO-compatible storage client (`internal/storage/s3`) + storage provider status & migration warning UI; implement `internal/email` (`net/smtp`, encrypted credentials, test connection, test email, sanitized `cms_email_templates` + variable preview).
   - **Milestone 5 (Security & Users & Permissions)**: Configurable Password Policy, Account Lockout, TOTP MFA setup/verification + backup codes; OAuth/OIDC provider config (`Google`, `GitHub`, `Microsoft`, generic OIDC) + SSO Admin policy; strictly separated Users & Permissions (`cms_app_roles` Public/Authenticated/Custom, `cms_app_users` management + block/confirm/reset, Providers, Email Templates, Advanced Auth settings).
3. **Pass 3 — Integrations, Audit & Full Verification (Milestones 6–7)**:
   - **Milestone 6 (Webhooks, Plugins, i18n)**: `internal/webhooks` with SSRF-safe dialer, HMAC-SHA256 signing, `EventBus` subscriber (`create`/`update`/`delete`/`publish`/`unpublish`/`media`), delivery history & retries, Test trigger + UI; persistent `cms_plugins_state` + schema-driven plugin settings + `/ext/*` permission enforcement; persistent `cms_locales` + deletion safety guard + Internationalization Settings UI.
   - **Milestone 7 (Audit Logs, E2E Security & Verification)**: Unified persistent `audit.SQLRepository` logging all Settings/Auth/RBAC/Token/Webhook/Media/Plugin changes with secret redaction; Audit Logs filterable UI + detail drawer + CSV export; comprehensive Go unit/integration tests, frontend tests, Live Chrome end-to-end verification across every workflow, regression check of CTB/CM/Media/Publish, and updated docs.

