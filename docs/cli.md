# Go CMS CLI Command Reference

The `go-cms` command-line interface provides tools for scaffolding, running, maintaining, and deploying Go CMS projects.

```text
Usage:
  go-cms <command> [options]
```

---

## Commands

### `go-cms create`
Scaffolds a new Go CMS project.

```bash
# Interactive mode
go-cms create my-project

# Non-interactive quickstart
go-cms create my-project --quickstart

# Specific mode and template
go-cms create my-api --mode=headless --template=blank
go-cms create my-blog --mode=templated --template=blog --db-port=5433
```

#### Flags:
* `--mode`: CMS mode (`templated` or `headless`). Defaults to `templated`.
* `--template`: Starter template (`blog` or `blank`). Defaults to `blog`.
* `--db`: Database driver (`postgres`). Defaults to `postgres`.
* `--db-host`: Database hostname. Defaults to `localhost`.
* `--db-port`: Database port. Defaults to `5432`.
* `--db-user`: Database user. Defaults to `postgres`.
* `--db-pass`: Database password.
* `--db-name`: Database name. Defaults to `<project-name>_db`.
* `--port`: HTTP server port. Defaults to `8080`.
* `--quickstart`, `-y`: Accept defaults non-interactively.

---

### `go-cms develop` (alias: `go-cms dev`)
Runs the CMS server in development mode.

```bash
go-cms develop
go-cms develop --config custom-config.yaml
```

* Loads `config.yaml` and `.env`.
* Runs auto-migrations on database startup.
* Seeds default roles and admin user if database is uninitialized.
* Disables aggressive template caching for rapid local styling.
* Displays formatted startup banner with direct URL links.
* Handles graceful shutdown on `Ctrl+C`.

---

### `go-cms start`
Runs the CMS server in hardened production mode.

```bash
go-cms start
```

* Enforces `CMS_SERVER_ENV=production`.
* Enables template caching for maximum throughput and sub-millisecond response times.
* Validates that production JWT secrets and database credentials are set.

---

### `go-cms build`
Validates the project for production deployment.

```bash
go-cms build
```

* Validates `config.yaml` structure and values.
* If `themes/` exists, parses and validates all Go HTML templates using `templates.SafeFuncMap` to guarantee zero syntax or partial reference errors.
* If `schemas/` exists, validates declarative JSON schemas.
* Returns exit code `0` on success, exit code `1` with descriptive errors on failure.

---

### `go-cms migrate`
Manages database schema migrations.

```bash
# Apply all pending migrations
go-cms migrate up

# Print status of applied and pending migrations
go-cms migrate status
```

---

### `go-cms doctor`
Runs comprehensive system diagnostics.

```bash
go-cms doctor
```

#### Diagnostics Performed:
1. **System Runtime:** Go version, operating system, CPU architecture.
2. **Configuration File:** Validates `config.yaml` and `.env` presence and syntax.
3. **Security Secrets:** Checks JWT and session secrets; warns if weak or default.
4. **Database Connection:** Pings the configured database server; reports connection latency.
5. **Port Availability:** Verifies whether the server port is open and bindable.
6. **Media Storage:** Tests write permissions on the `uploads/` directory.
7. **Theme System:** Validates that the active theme directory exists.
8. **Admin Console Assets:** Verifies embedded React Admin SPA integrity.

---

### `go-cms version` (alias: `-v`, `--version`)
Prints the current Go CMS CLI version, commit hash, build date, Go runtime, and license.

```bash
go-cms version
```

