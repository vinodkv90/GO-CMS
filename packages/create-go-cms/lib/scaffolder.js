const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const { getPlatformInfo } = require('./platform');
const { ensureBinary } = require('./downloader');

function randomHex(length = 32) {
  return crypto.randomBytes(length).toString('hex');
}

async function scaffoldProject(options) {
  const {
    projectName,
    targetDir = path.resolve(process.cwd(), projectName),
    mode = 'templated',
    template = 'blog',
    dbHost = 'localhost',
    dbPort = 5432,
    dbUser = 'postgres',
    dbPass = '',
    dbName = `${projectName.replace(/[^a-zA-Z0-9]/g, '_')}_db`,
    serverPort = 8080
  } = options;

  console.log(`\n⏳ Scaffolding Go CMS project in ${targetDir}...`);
  fs.mkdirSync(targetDir, { recursive: true });

  const platformInfo = getPlatformInfo();

  // 1. Ensure binary is present in project bin/
  const projectBinDir = path.join(targetDir, 'bin');
  await ensureBinary(projectBinDir, platformInfo);

  // 2. Ensure go-cms-runner.js is written into scripts/
  const projectScriptsDir = path.join(targetDir, 'scripts');
  fs.mkdirSync(projectScriptsDir, { recursive: true });
  const runnerDest = path.join(projectScriptsDir, 'go-cms-runner.js');
  const runnerSource = path.resolve(__dirname, '../scripts/go-cms-runner.js');
  if (fs.existsSync(runnerSource)) {
    fs.copyFileSync(runnerSource, runnerDest);
  } else {
    const inlineRunner = `#!/usr/bin/env node
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
    if (fs.existsSync(c)) return c;
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
    console.error('\\n❌ Error: Go CMS executable not found.');
    console.error('Please ensure the go-cms binary exists in bin/ or run:');
    console.error('  go install github.com/vinodkv90/GO-CMS/cmd/go-cms@latest\\n');
  } else {
    console.error('Failed to execute Go CMS:', err.message);
  }
  process.exit(1);
});

child.on('close', (code) => {
  process.exit(code ?? 0);
});
`;
    fs.writeFileSync(runnerDest, inlineRunner, { mode: 0o755 });
  }

  // 3. Create subdirectories
  const subdirs = ['uploads', 'plugins', 'schemas'];
  if (mode === 'templated') subdirs.push('themes');
  for (const d of subdirs) {
    const dirPath = path.join(targetDir, d);
    fs.mkdirSync(dirPath, { recursive: true });
    fs.writeFileSync(path.join(dirPath, '.gitkeep'), '');
  }

  // 4. Generate secrets
  const jwtSecret = randomHex(32);
  const sessionSecret = randomHex(32);
  const mediaSecret = randomHex(32);

  // 5. Write config.yaml
  const configContent = `# Go CMS Configuration
# Documentation: https://github.com/go-cms/cms

server:
  host: "0.0.0.0"
  port: ${serverPort}
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
  driver: "postgres"
  host: "${dbHost}"
  port: ${dbPort}
  user: "${dbUser}"
  password: "${dbPass}"
  database: "${dbName}"
  ssl_mode: "disable"
  max_open_conns: 25
  max_idle_conns: 10
  conn_max_lifetime: 30m
  conn_max_idle_time: 5m

logging:
  level: "info"
  format: "text"

auth:
  jwt_secret: "\${CMS_AUTH_JWT_SECRET:-${jwtSecret}}"
  session_secret: "\${CMS_AUTH_SESSION_SECRET:-${sessionSecret}}"
  token_expiration: 24h

media:
  provider: "local"
  upload_dir: "./uploads"
  signed_urls:
    enabled: true
    algorithm: "HMAC-SHA256"
    expiration: 1h
    secret_key: "\${CMS_MEDIA_SIGNED_URLS_SECRET_KEY:-${mediaSecret}}"

themes:
  active_theme: "default"
  themes_dir: "./themes"
  cache: false

plugins:
  plugins_dir: "./plugins"
`;
  fs.writeFileSync(path.join(targetDir, 'config.yaml'), configContent);

  // 6. Write .env
  const envContent = `# Go CMS Environment Variables
CMS_SERVER_HOST=0.0.0.0
CMS_SERVER_PORT=${serverPort}
CMS_SERVER_ENV=development

CMS_DB_DRIVER=postgres
CMS_DB_HOST=${dbHost}
CMS_DB_PORT=${dbPort}
CMS_DB_USER=${dbUser}
CMS_DB_PASSWORD=${dbPass}
CMS_DB_NAME=${dbName}
CMS_DB_SSLMODE=disable

CMS_AUTH_JWT_SECRET=${jwtSecret}
CMS_AUTH_SESSION_SECRET=${sessionSecret}
CMS_MEDIA_SIGNED_URLS_SECRET_KEY=${mediaSecret}
`;
  fs.writeFileSync(path.join(targetDir, '.env'), envContent);

  // 7. Write package.json with working npm scripts
  const packageJSON = {
    name: projectName,
    version: '0.1.0',
    private: true,
    description: `Go CMS ${mode} application`,
    scripts: {
      dev: 'node scripts/go-cms-runner.js develop',
      develop: 'node scripts/go-cms-runner.js develop',
      start: 'node scripts/go-cms-runner.js start',
      build: 'node scripts/go-cms-runner.js build',
      migrate: 'node scripts/go-cms-runner.js migrate up',
      doctor: 'node scripts/go-cms-runner.js doctor'
    },
    keywords: ['go-cms', 'cms', 'strapi', 'nextjs', 'gutenberg']
  };
  fs.writeFileSync(path.join(targetDir, 'package.json'), JSON.stringify(packageJSON, null, 2) + '\n');

  // 8. Write .gitignore
  const gitignore = `# Dependencies and binaries
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

# Logs
*.log
`;
  fs.writeFileSync(path.join(targetDir, '.gitignore'), gitignore);

  // 9. Write README.md
  const readme = `# ${projectName}

Welcome to your new **Go CMS** project!

Go CMS is a high-performance open-source content management system inspired by Strapi and WordPress Gutenberg, built with Go and React.

## Quickstart

Run the development server:

\`\`\`bash
npm run dev
\`\`\`

Once running, access your application:

* **Admin Dashboard:** [http://localhost:${serverPort}/admin/](http://localhost:${serverPort}/admin/)
* **Public Website:** [http://localhost:${serverPort}/](http://localhost:${serverPort}/)
* **REST APIs:** [http://localhost:${serverPort}/api/v1](http://localhost:${serverPort}/api/v1)
* **System Health:** [http://localhost:${serverPort}/healthz](http://localhost:${serverPort}/healthz)

Default Administrator:
* **Username:** \`admin\`
* **Password:** \`Admin@CMS2026!\`

## Available Scripts

* \`npm run dev\`: Starts the server in development mode with auto-migrations.
* \`npm run build\`: Validates themes, schemas, and configuration for production.
* \`npm run start\`: Runs the server in hardened production mode.
* \`npm run migrate\`: Runs pending database migrations.
* \`npm run doctor\`: Runs diagnostics on your environment and database.

---
MIT Licensed • Powered by [Go CMS](https://github.com/go-cms/cms)
`;
  fs.writeFileSync(path.join(targetDir, 'README.md'), readme);

  // 10. If templated mode, copy theme assets if available in monorepo
  if (mode === 'templated') {
    const localThemesDir = path.resolve(__dirname, '../../../themes/default');
    if (fs.existsSync(localThemesDir)) {
      copyRecursive(localThemesDir, path.join(targetDir, 'themes', 'default'));
    }
  }

  // Success Banner
  console.log(`
✨ Success! Created ${projectName} at ${targetDir}
Inside that directory, you can run several commands:

  npm run dev
    Starts the development server with auto-migrations.

  npm run build
    Validates themes, schemas, and configuration for production.

  npm run start
    Runs the hardened server in production mode.

  npm run migrate
    Applies pending database migrations.

  npm run doctor
    Runs system diagnostics on your database and configuration.

We suggest that you begin by typing:

  cd ${projectName}
  npm run dev

Happy hacking with Go CMS! 🚀
`);
}

function copyRecursive(src, dest) {
  if (fs.statSync(src).isDirectory()) {
    fs.mkdirSync(dest, { recursive: true });
    for (const file of fs.readdirSync(src)) {
      copyRecursive(path.join(src, file), path.join(dest, file));
    }
  } else {
    fs.mkdirSync(path.dirname(dest), { recursive: true });
    fs.copyFileSync(src, dest);
  }
}

module.exports = { scaffoldProject };

