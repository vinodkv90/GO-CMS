package scaffold

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/go-cms/cms/themes"
)

// ProjectMode defines the CMS operational mode.
type ProjectMode string

const (
	ModeTemplated ProjectMode = "templated"
	ModeHeadless  ProjectMode = "headless"
)

// ProjectTemplate defines the starter template type.
type ProjectTemplate string

const (
	TemplateBlog  ProjectTemplate = "blog"
	TemplateBlank ProjectTemplate = "blank"
)

// Options holds project scaffolding parameters.
type Options struct {
	ProjectName string
	TargetDir   string
	Mode        ProjectMode
	Template    ProjectTemplate
	DBDriver    string
	DBHost      string
	DBPort      int
	DBUser      string
	DBPassword  string
	DBName      string
	ServerPort  int
}

// Generate creates the project file structure at targetDir.
func Generate(opts Options) error {
	if opts.TargetDir == "" {
		opts.TargetDir = opts.ProjectName
	}
	if opts.Mode == "" {
		opts.Mode = ModeTemplated
	}
	if opts.Template == "" {
		opts.Template = TemplateBlog
	}
	if opts.DBDriver == "" {
		opts.DBDriver = "postgres"
	}
	if opts.DBHost == "" {
		opts.DBHost = "localhost"
	}
	if opts.DBPort == 0 {
		opts.DBPort = 5432
	}
	if opts.DBUser == "" {
		opts.DBUser = "postgres"
	}
	if opts.DBName == "" {
		cleanName := strings.ReplaceAll(opts.ProjectName, "-", "_")
		opts.DBName = cleanName + "_db"
	}
	if opts.ServerPort == 0 {
		opts.ServerPort = 8080
	}

	// 1. Create target project directory
	if err := os.MkdirAll(opts.TargetDir, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// 2. Generate cryptographically secure random secrets
	jwtSecret := randomHex(32)
	sessionSecret := randomHex(32)
	mediaSecret := randomHex(32)

	// 3. Create subdirectories
	subdirs := []string{"uploads", "plugins", "schemas", "scripts", "bin"}
	if opts.Mode == ModeTemplated {
		subdirs = append(subdirs, "themes")
	}
	for _, d := range subdirs {
		dirPath := filepath.Join(opts.TargetDir, d)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return fmt.Errorf("failed to create directory %q: %w", d, err)
		}
		_ = os.WriteFile(filepath.Join(dirPath, ".gitkeep"), []byte(""), 0644)
	}

	// 3b. Copy current executable to target project bin/ if available
	if currentExe, err := os.Executable(); err == nil {
		binName := "go-cms"
		if runtime.GOOS == "windows" {
			binName = "go-cms.exe"
		}
		destExe := filepath.Join(opts.TargetDir, "bin", binName)
		if srcF, err := os.Open(currentExe); err == nil {
			if dstF, err := os.OpenFile(destExe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755); err == nil {
				_, _ = io.Copy(dstF, srcF)
				dstF.Close()
			}
			srcF.Close()
		}
	}

	// 3c. Write cross-platform runner script in scripts/go-cms-runner.js
	runnerScript := `#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const { spawn } = require('child_process');

const isWindows = process.platform === 'win32';
const binName = isWindows ? 'go-cms.exe' : 'go-cms';

function resolveBinary() {
  const projectRoot = path.resolve(__dirname, '..');
  const candidates = [
    path.join(projectRoot, 'bin', binName),
    path.join(projectRoot, 'node_modules', '.bin', binName),
    path.join(projectRoot, '..', 'bin', binName)
  ];

  for (const c of candidates) {
    if (fs.existsSync(c)) {
      return c;
    }
  }

  return 'go-cms';
}

const binary = resolveBinary();
const args = process.argv.slice(2);

const child = spawn(binary, args, {
  stdio: 'inherit',
  env: process.env,
  shell: binary === 'go-cms' ? isWindows : false
});

child.on('error', (err) => {
  if (err.code === 'ENOENT') {
    console.error('\n❌ Error: Go CMS executable not found.');
    console.error('Please ensure the go-cms binary exists in bin/ or run:');
    console.error('  go install github.com/go-cms/cms/cmd/go-cms@latest\n');
  } else {
    console.error('Failed to execute Go CMS:', err.message);
  }
  process.exit(1);
});

child.on('close', (code) => {
  process.exit(code ?? 0);
});
`
	_ = os.WriteFile(filepath.Join(opts.TargetDir, "scripts", "go-cms-runner.js"), []byte(runnerScript), 0755)

	// 4. Write config.yaml
	configContent := fmt.Sprintf(`# Go CMS Configuration
# Documentation: https://github.com/go-cms/cms

server:
  host: "0.0.0.0"
  port: %d
  env: "development"
  read_timeout: 15s
  write_timeout: 30s
  idle_timeout: 60s
  shutdown_timeout: 10s
  cors_allowed_origins:
    - "*"
  trusted_proxies:
    - "127.0.0.1/32"
    - "::1/128"

database:
  driver: "%s"
  host: "%s"
  port: %d
  user: "%s"
  password: "%s"
  database: "%s"
  ssl_mode: "disable"
  max_open_conns: 25
  max_idle_conns: 10
  conn_max_lifetime: 30m
  conn_max_idle_time: 5m

logging:
  level: "info"
  format: "text"

auth:
  jwt_secret: "${CMS_AUTH_JWT_SECRET:-%s}"
  session_secret: "${CMS_AUTH_SESSION_SECRET:-%s}"
  token_expiration: 24h

media:
  provider: "local"
  upload_dir: "./uploads"
  signed_urls:
    enabled: true
    algorithm: "HMAC-SHA256"
    expiration: 1h
    secret_key: "${CMS_MEDIA_SIGNED_URLS_SECRET_KEY:-%s}"

themes:
  active_theme: "default"
  themes_dir: "./themes"
  cache: false

plugins:
  plugins_dir: "./plugins"
`, opts.ServerPort, opts.DBDriver, opts.DBHost, opts.DBPort, opts.DBUser, opts.DBPassword, opts.DBName, jwtSecret, sessionSecret, mediaSecret)

	if err := os.WriteFile(filepath.Join(opts.TargetDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write config.yaml: %w", err)
	}

	// 5. Write .env
	envContent := fmt.Sprintf(`# Go CMS Environment Variables
# Auto-generated on project scaffolding

CMS_SERVER_HOST=0.0.0.0
CMS_SERVER_PORT=%d
CMS_SERVER_ENV=development

CMS_DB_DRIVER=%s
CMS_DB_HOST=%s
CMS_DB_PORT=%d
CMS_DB_USER=%s
CMS_DB_PASSWORD=%s
CMS_DB_NAME=%s
CMS_DB_SSLMODE=disable

CMS_AUTH_JWT_SECRET=%s
CMS_AUTH_SESSION_SECRET=%s
CMS_MEDIA_SIGNED_URLS_SECRET_KEY=%s
`, opts.ServerPort, opts.DBDriver, opts.DBHost, opts.DBPort, opts.DBUser, opts.DBPassword, opts.DBName, jwtSecret, sessionSecret, mediaSecret)

	if err := os.WriteFile(filepath.Join(opts.TargetDir, ".env"), []byte(envContent), 0600); err != nil {
		return fmt.Errorf("failed to write .env: %w", err)
	}

	// 6. Write package.json
	packageJSON := fmt.Sprintf(`{
  "name": "%s",
  "version": "0.1.0",
  "private": true,
  "description": "Go CMS %s project",
  "scripts": {
    "dev": "node scripts/go-cms-runner.js develop",
    "develop": "node scripts/go-cms-runner.js develop",
    "start": "node scripts/go-cms-runner.js start",
    "build": "node scripts/go-cms-runner.js build",
    "migrate": "node scripts/go-cms-runner.js migrate up",
    "doctor": "node scripts/go-cms-runner.js doctor"
  },
  "keywords": [
    "go-cms",
    "cms",
    "strapi",
    "nextjs",
    "gutenberg"
  ]
}
`, opts.ProjectName, opts.Mode)

	if err := os.WriteFile(filepath.Join(opts.TargetDir, "package.json"), []byte(packageJSON), 0644); err != nil {
		return fmt.Errorf("failed to write package.json: %w", err)
	}

	// 7. Write .gitignore
	gitignore := `# Dependencies and binaries
node_modules/
bin/
*.exe

# CMS Data & uploads
uploads/
data/
*.db
*.sqlite

# Environment secrets
.env
.env.local
.env.*.local

# Logs
*.log
`
	if err := os.WriteFile(filepath.Join(opts.TargetDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		return fmt.Errorf("failed to write .gitignore: %w", err)
	}

	// 8. Write README.md
	portStr := strconv.Itoa(opts.ServerPort)
	if opts.ServerPort <= 0 {
		portStr = "8080"
	}

	modeTitle := string(opts.Mode)
	if len(modeTitle) > 0 {
		modeTitle = strings.ToUpper(modeTitle[:1]) + modeTitle[1:]
	}

	readme := fmt.Sprintf(`# %s

Welcome to your new **Go CMS** project!

Go CMS is a high-performance open-source content management system inspired by Strapi and WordPress Gutenberg, built with Go and React.

## Project Mode: %s

%s

## Quickstart

Run the development server:

`+"```bash"+`
npm run dev
`+"```"+`

Or using the native Go CLI:

`+"```bash"+`
go-cms develop
`+"```"+`

Once running, access your application:

* **Admin Dashboard:** [http://localhost:%s/admin/](http://localhost:%s/admin/)
* **Public Website:** [http://localhost:%s/](http://localhost:%s/)
* **REST APIs:** [http://localhost:%s/api/v1](http://localhost:%s/api/v1)
* **System Health:** [http://localhost:%s/healthz](http://localhost:%s/healthz)

Default Admin Credentials:
* **Username:** `+"`admin`"+`
* **Password:** `+"`Admin@CMS2026!`"+`

## Available Scripts

In the project directory, you can run:

* `+"`npm run dev`"+`: Starts the server in development mode with auto-migrations.
* `+"`npm run build`"+`: Validates themes, schemas, and configuration for production.
* `+"`npm run start`"+`: Runs the server in hardened production mode.
* `+"`npm run migrate`"+`: Runs pending database migrations.
* `+"`npm run doctor`"+`: Runs diagnostic checks on your environment, database, and ports.

## Project Structure

* `+"`config.yaml`"+`: Server, database, media, and security configuration.
* `+"`.env`"+`: Environment secrets and local overrides.
* `+"`themes/`"+`: Server-rendered HTML templates and CSS/JS assets.
* `+"`plugins/`"+`: CMS extension plugins.
* `+"`schemas/`"+`: Custom content-type schema definitions.
* `+"`uploads/`"+`: Locally stored media assets.

---
MIT Licensed &bull; Powered by [Go CMS](https://github.com/go-cms/cms)
`, opts.ProjectName, modeTitle, getModeDescription(opts.Mode), portStr, portStr, portStr, portStr, portStr, portStr, portStr, portStr)

	if err := os.WriteFile(filepath.Join(opts.TargetDir, "README.md"), []byte(readme), 0644); err != nil {
		return fmt.Errorf("failed to write README.md: %w", err)
	}

	// 9. If Templated mode, extract the embedded default theme
	if opts.Mode == ModeTemplated {
		themeFS, err := themes.GetDefaultThemeFS()
		if err != nil {
			return fmt.Errorf("failed to load embedded default theme: %w", err)
		}
		destDir := filepath.Join(opts.TargetDir, "themes", "default")
		if err := copyFSToDisk(themeFS, destDir); err != nil {
			return fmt.Errorf("failed to extract default theme: %w", err)
		}
	}

	return nil
}

func getModeDescription(m ProjectMode) string {
	if m == ModeHeadless {
		return "This project is configured as a **Headless CMS**. Content is authored in the React Admin Console and delivered to external applications (Next.js, React, mobile apps) via JSON REST APIs."
	}
	return "This project is configured as a **Templated CMS**. Pages are rendered server-side by Go's native HTML template engine and designed with the WordPress Gutenberg-style visual Page Builder."
}

func randomHex(bytesLen int) string {
	b := make([]byte, bytesLen)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func copyFSToDisk(srcFS fs.FS, destDir string) error {
	return fs.WalkDir(srcFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := fs.ReadFile(srcFS, path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}
