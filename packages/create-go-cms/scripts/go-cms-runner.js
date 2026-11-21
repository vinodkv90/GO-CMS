#!/usr/bin/env node

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

  // Fallback to global PATH
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
    console.error(`\n❌ Error: Go CMS executable not found.`);
    console.error(`Please install the native CLI binary via:`);
    console.error(`  go install github.com/go-cms/cms/cmd/go-cms@latest`);
    console.error(`Or run: npx create-go-cms --doctor\n`);
  } else {
    console.error(`Failed to execute Go CMS:`, err.message);
  }
  process.exit(1);
});

child.on('close', (code) => {
  process.exit(code ?? 0);
});
