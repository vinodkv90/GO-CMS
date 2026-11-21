const os = require('os');

function getPlatformInfo() {
  const rawPlatform = os.platform();
  const rawArch = os.arch();

  let platform = '';
  switch (rawPlatform) {
    case 'win32':
      platform = 'windows';
      break;
    case 'darwin':
      platform = 'darwin';
      break;
    case 'linux':
      platform = 'linux';
      break;
    default:
      throw new Error(`Unsupported operating system: ${rawPlatform}. Go CMS supports Windows, macOS, and Linux.`);
  }

  let arch = '';
  switch (rawArch) {
    case 'x64':
      arch = 'amd64';
      break;
    case 'arm64':
      arch = 'arm64';
      break;
    default:
      throw new Error(`Unsupported CPU architecture: ${rawArch}. Go CMS supports x64 (amd64) and arm64.`);
  }

  const isWindows = platform === 'windows';
  const binaryName = isWindows ? 'go-cms.exe' : 'go-cms';
  const archiveExt = isWindows ? 'zip' : 'tar.gz';

  return {
    platform,
    arch,
    isWindows,
    binaryName,
    archiveExt,
    targetAsset: `go-cms_${platform}_${arch}.${archiveExt}`
  };
}

module.exports = { getPlatformInfo };

