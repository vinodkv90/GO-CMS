# Security Architecture & Verification Specification

## 1. Compliance Baseline

The CMS architecture is verified against **OWASP Application Security Verification Standard (ASVS) Level 2** and the **OWASP API Security Top 10**.

Every request entering the system must pass through strict defense-in-depth controls before reaching business logic or database layers.

---

## 2. Threat Mitigations & Controls

### 2.1 Injection Attacks (SQL & NoSQL)
- **Parameterized Queries:** All SQL queries (PostgreSQL and MySQL) use native parameterized placeholders (`$1, $2` for PostgreSQL; `?` for MySQL). String concatenation in SQL is strictly prohibited and guarded against with static analysis.
- **NoSQL Sanity:** MongoDB queries use strongly typed `bson.M` and `bson.D` builders. User input is never parsed directly as raw BSON/JSON query syntax.
- **Identifier Whitelisting:** Dynamic table names, column sorting keys, and collection names are strictly validated against an in-memory whitelist of registered schemas.

### 2.2 Template Injection & Cross-Site Scripting (XSS)
- **Standard Library `html/template`:** Go's `html/template` provides context-aware contextual auto-escaping.
- **No Direct HTML Bypasses:** Content marked with `template.HTML` is forbidden for untrusted user input without explicit pre-sanitization using a strict HTML sanitizer (such as `bluemonday` configured to an allowlist).
- **Function Allowlist:** Only pure, safe helper functions are registered in the global `template.FuncMap`. Filesystem operations, command executions, and network primitives are never exposed to templates.

### 2.3 Cross-Site Request Forgery (CSRF) & Cookie Security
- **SameSite Cookies:** Authentication cookies are configured with `SameSite=Lax` or `SameSite=Strict`, `HttpOnly=true`, and `Secure=true` in production environments.
- **Double Submit / Synchronizer Token:** State-changing requests (POST, PUT, DELETE, PATCH) using cookie authentication require an anti-CSRF token validated via middleware.

### 2.4 Server-Side Request Forgery (SSRF) Protection
- Used by: Webhooks, dynamic media fetchers, oEmbed providers.
- **Safe Transport Client:** All outbound HTTP requests pass through an SSRF-safe dialer that:
  1. Resolves DNS hostnames and verifies all resolved IP addresses.
  2. Denies private, loopback, link-local, multicast, and carrier-grade NAT IP ranges (RFC 1918, RFC 3927, RFC 4193, RFC 4291, RFC 5737, RFC 6598).
  3. Prohibits loopback DNS re-binding by re-verifying the IP address upon socket connection.
  4. Restricts protocols strictly to `http` and `https` and enforces connection timeouts.

### 2.5 Path Traversal & Arbitrary File Uploads
- **Filename Normalization:** Uploaded filenames are sanitized: spaces replaced with hyphens (`-`), non-alphanumeric/dot characters removed, leading dots removed, and length restricted.
- **Safe Storage Keys:** Files are saved using UUID-based storage keys or sanitized hashes. Path separators (`/`, `\`, `..`) are completely stripped.
- **MIME Sniffing & Validation:** Magic bytes are inspected (`http.DetectContentType`) rather than trusting the user-supplied `Content-Type` header.
- **Restricted Execution:** Storage directories never execute scripts. When using local storage, uploaded files are served with `Content-Disposition: attachment` or strict `Content-Type` with sandboxed headers.

### 2.6 Signed URL Algorithm
For private assets and controlled delivery:
1. When signed URLs are enabled:
   - Filename is rendered with a secure hash: `<basename>-<random_hex_8>.<ext>`.
   - The URL includes signature query parameters:
     `?expires=<unix_timestamp>&id=<asset_id>&sig=<hmac_sha256_signature>`
   - The signature is calculated as:
     `HMAC-SHA256(secret_key, asset_id + ":" + expires + ":" + tenant_id)`
2. The verification middleware validates:
   - Current time is strictly less than `expires`.
   - Constant-time HMAC comparison prevents timing attacks (`hmac.Equal`).
   - Tenant isolation matches the requesting context.

---

## 3. HTTP Security Headers & WAF Configuration

Every response from the CMS server emits standard hardened HTTP headers:

```http
Content-Security-Policy: default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'self';
X-Content-Type-Options: nosniff
X-Frame-Options: SAMEORIGIN
X-XSS-Protection: 1; mode=block
Referrer-Policy: strict-origin-when-cross-origin
Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
```

### Trusted Proxies & WAF Compatibility
- Works seamlessly behind Cloudflare WAF, AWS WAF, Azure WAF, and Nginx.
- Client IP resolution never trusts `X-Forwarded-For` blindly. Only configured trusted CIDRs (e.g. reverse proxy subnet) are allowed to populate the real client IP context.

---

## 4. Authentication & RBAC

- **Argon2id Password Hashing:** Memory: 64MB, Iterations: 3, Parallelism: 2.
- **Granular Permissions:** Every resource is evaluated against user roles (`content:articles:read`, `content:articles:publish`, `media:upload`, etc.).
- **Account Lockout & Rate Limiting:** Progressive delay and account lockout after repeated failed login attempts.
