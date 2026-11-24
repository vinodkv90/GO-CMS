#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const { spawn, execSync } = require('child_process');

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
      try {
        const stats = fs.statSync(c);
        if (stats.size > 0) {
          return c;
        }
      } catch {}
    }
  }

  try {
    const whichCmd = isWindows ? 'where go-cms' : 'which go-cms';
    const found = execSync(whichCmd, { stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim().split('\n')[0].trim();
    if (found && fs.existsSync(found)) {
      return found;
    }
  } catch {}

  return null;
}

const binary = resolveBinary();
const args = process.argv.slice(2);

if (!binary) {
  console.error('\n❌ Error: Go CMS executable not found.');
  console.error(`Expected native binary at: ${path.resolve(__dirname, '..', 'bin', binName)}`);
  console.error('\nTo resolve this issue:');
  console.error('  1. Download the prebuilt binary from:');
  console.error('     https://github.com/vinodkv90/GO-CMS/releases');
  console.error("     and place it into your project's bin/ directory.");
  console.error('  2. Or compile locally using Go:');
  console.error('     go install github.com/vinodkv90/GO-CMS/cmd/go-cms@latest\n');
  process.exit(1);
}

const child = spawn(binary, args, {
  stdio: 'inherit',
  env: process.env,
  shell: false
});

child.on('error', (err) => {
  console.error('Failed to execute Go CMS:', err.message);
  process.exit(1);
});

child.on('close', (code) => {
  process.exit(code ?? 0);
});
