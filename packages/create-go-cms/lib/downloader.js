const fs = require('fs');
const path = require('path');
const https = require('https');
const crypto = require('crypto');
const { execSync } = require('child_process');

const REPO_OWNER = process.env.GO_CMS_REPO_OWNER || 'vinodkv90';
const REPO_NAME = process.env.GO_CMS_REPO_NAME || 'GO-CMS';
const DEFAULT_VERSION = '0.1.0';

/**
 * Checks if a valid go-cms binary is already present locally.
 */
function findLocalBinary(binaryName) {
  // 1. Check parent repository bin folder (development mode) and GOPATH bin
  const userHome = process.env.USERPROFILE || process.env.HOME || '';
  const gopath = process.env.GOPATH || (userHome ? path.join(userHome, 'go') : '');

  const candidateDevPaths = [
    path.resolve(__dirname, '../../../bin', binaryName),
    path.resolve(__dirname, '../../bin', binaryName),
    path.resolve(process.cwd(), 'bin', binaryName),
  ];

  if (gopath) {
    candidateDevPaths.push(path.join(gopath, 'bin', binaryName));
  }

  for (const c of candidateDevPaths) {
    if (fs.existsSync(c)) {
      try {
        fs.accessSync(c, fs.constants.X_OK || fs.constants.R_OK);
        return c;
      } catch {}
    }
  }

  // 2. Check if go-cms is in system PATH
  try {
    const whichCmd = process.platform === 'win32' ? 'where' : 'which';
    const found = execSync(`${whichCmd} go-cms`, { stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim().split('\n')[0].trim();
    if (found && fs.existsSync(found)) {
      return found;
    }
  } catch {}

  return null;
}

/**
 * Downloads a file from a URL using native https with redirect following.
 */
function downloadFile(url, destPath) {
  return new Promise((resolve, reject) => {
    const urlObj = new URL(url);
    const headers = {
      'User-Agent': 'create-go-cms'
    };
    const ghToken = process.env.GITHUB_TOKEN || process.env.GH_TOKEN;
    if (ghToken && urlObj.hostname === 'github.com') {
      headers['Authorization'] = `Bearer ${ghToken}`;
    }

    const request = https.get(url, { headers }, (response) => {
      // Follow HTTP 301/302 redirects
      if (response.statusCode >= 300 && response.statusCode < 400 && response.headers.location) {
        return resolve(downloadFile(response.headers.location, destPath));
      }

      if (response.statusCode !== 200) {
        return reject(new Error(`Download failed with HTTP ${response.statusCode} from ${url}`));
      }

      const fileStream = fs.createWriteStream(destPath);
      response.pipe(fileStream);

      fileStream.on('finish', () => {
        fileStream.close(resolve);
      });
      fileStream.on('error', (err) => {
        fs.unlink(destPath, () => {});
        reject(err);
      });
    });

    request.on('error', (err) => {
      fs.unlink(destPath, () => {});
      reject(err);
    });
  });
}
  });
}

/**
 * Verifies SHA-256 hash of a file against expected checksum.
 */
function verifyChecksum(filePath, expectedSha256) {
  const fileBuffer = fs.readFileSync(filePath);
  const hashSum = crypto.createHash('sha256').update(fileBuffer).digest('hex');
  return hashSum.toLowerCase() === expectedSha256.toLowerCase();
}

/**
 * Ensures the go-cms binary is available in destDir.
 */
async function ensureBinary(destDir, platformInfo, version = DEFAULT_VERSION) {
  const targetBinaryPath = path.join(destDir, platformInfo.binaryName);
  const localBinary = findLocalBinary(platformInfo.binaryName);

  if (localBinary) {
    // Copy local binary to project bin/
    fs.mkdirSync(destDir, { recursive: true });
    fs.copyFileSync(localBinary, targetBinaryPath);
    if (!platformInfo.isWindows) {
      fs.chmodSync(targetBinaryPath, 0o755);
    }
    return targetBinaryPath;
  }

  // Download from GitHub release
  const releaseBaseUrl = `https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/v${version}`;
  const archiveName = `go-cms_${version}_${platformInfo.platform}_${platformInfo.arch}.${platformInfo.archiveExt}`;
  const archiveUrl = `${releaseBaseUrl}/${archiveName}`;
  const checksumUrl = `${releaseBaseUrl}/checksums.txt`;

  fs.mkdirSync(destDir, { recursive: true });
  const tempArchive = path.join(destDir, archiveName);
  const tempChecksums = path.join(destDir, 'checksums.txt');

  console.log(`⬇️  Downloading Go CMS ${version} binary for ${platformInfo.platform}/${platformInfo.arch}...`);

  try {
    await Promise.all([
      downloadFile(archiveUrl, tempArchive),
      downloadFile(checksumUrl, tempChecksums)
    ]);

    // Parse checksums.txt
    const checksumContent = fs.readFileSync(tempChecksums, 'utf8');
    let expectedSha256 = '';
    for (const line of checksumContent.split('\n')) {
      const parts = line.trim().split(/\s+/);
      if (parts.length >= 2 && parts[1].includes(archiveName)) {
        expectedSha256 = parts[0];
        break;
      }
    }

    if (expectedSha256) {
      const valid = verifyChecksum(tempArchive, expectedSha256);
      if (!valid) {
        throw new Error(`SECURITY ALERT: Checksum verification failed for ${archiveName}. Binary may be corrupted or tampered with.`);
      }
      console.log('🔒 Checksum verified successfully.');
    }

    // Extract archive
    if (platformInfo.isWindows) {
      execSync(`tar -xf "${tempArchive}" -C "${destDir}"`);
    } else {
      execSync(`tar -xzf "${tempArchive}" -C "${destDir}"`);
      fs.chmodSync(targetBinaryPath, 0o755);
    }

    // Clean up archive files
    fs.unlinkSync(tempArchive);
    fs.unlinkSync(tempChecksums);

    return targetBinaryPath;
  } catch (err) {
    if (fs.existsSync(tempArchive)) fs.unlinkSync(tempArchive);
    if (fs.existsSync(tempChecksums)) fs.unlinkSync(tempChecksums);

    // Fallback: If Go is installed locally, attempt to build or install it on the fly
    try {
      const whichCmd = platformInfo.isWindows ? 'where go' : 'which go';
      execSync(whichCmd, { stdio: ['ignore', 'ignore', 'ignore'] });
      console.log('⚡ Attempting local build with Go toolchain...');

      const userHome = process.env.USERPROFILE || process.env.HOME || '';
      const gopath = process.env.GOPATH || (userHome ? path.join(userHome, 'go') : '');
      const goInstalledBin = gopath ? path.join(gopath, 'bin', platformInfo.binaryName) : '';

      if (goInstalledBin && fs.existsSync(goInstalledBin)) {
        fs.copyFileSync(goInstalledBin, targetBinaryPath);
        return targetBinaryPath;
      }

      execSync(`go install github.com/${REPO_OWNER}/${REPO_NAME}/cmd/go-cms@latest`, { stdio: 'inherit' });
      if (goInstalledBin && fs.existsSync(goInstalledBin)) {
        fs.copyFileSync(goInstalledBin, targetBinaryPath);
        return targetBinaryPath;
      }
    } catch {}

    throw new Error(
      `Failed to install Go CMS binary: ${err.message}\n` +
      `You can build or install the native binary manually:\n` +
      `  go install github.com/${REPO_OWNER}/${REPO_NAME}/cmd/go-cms@latest\n` +
      `or copy go-cms into your project's bin/ folder.`
    );
  }
}

module.exports = {
  findLocalBinary,
  downloadFile,
  verifyChecksum,
  ensureBinary
};

