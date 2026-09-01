#!/usr/bin/env node
const fs = require('fs');
const path = require('path');
const https = require('https');
const http = require('http');

const GITHUB_REPO = 'bobbyunknown/flamegate';

const PLATFORM_MAP = {
  darwin: 'darwin',
  linux: 'linux',
  win32: 'windows',
};

const ARCH_MAP = {
  x64: 'amd64',
  arm64: 'arm64',
};

function getPlatform() {
  const os = PLATFORM_MAP[process.platform];
  const arch = ARCH_MAP[process.arch];

  if (!os || !arch) {
    console.error(
      `[flamegate] Unsupported platform: ${process.platform} (${process.arch}). ` +
        `FlameGate supports macOS, Linux, and Windows on amd64 and arm64.`
    );
    process.exit(1);
  }

  return { os, arch };
}

function downloadFile(url, dest) {
  return new Promise((resolve, reject) => {
    const client = url.startsWith('https') ? https : http;
    const request = client.get(url, (response) => {
      // Handle redirects (e.g. GitHub Releases 302 to S3/CDN)
      if (
        response.statusCode >= 300 &&
        response.statusCode < 400 &&
        response.headers.location
      ) {
        return downloadFile(response.headers.location, dest)
          .then(resolve)
          .catch(reject);
      }

      if (response.statusCode !== 200) {
        return reject(
          new Error(`Failed to download binary: HTTP ${response.statusCode}`)
        );
      }

      const fileStream = fs.createWriteStream(dest);
      response.pipe(fileStream);

      fileStream.on('finish', () => {
        fileStream.close();
        resolve();
      });

      fileStream.on('error', (err) => {
        fs.unlink(dest, () => reject(err));
      });
    });

    request.on('error', (err) => {
      fs.unlink(dest, () => reject(err));
    });
  });
}

async function install() {
  const { os, arch } = getPlatform();
  const ext = os === 'windows' ? '.exe' : '';
  const assetName = `flamegate-${os}-${arch}${ext}`;
  const binDir = path.join(__dirname, '..', 'bin');
  const targetBinPath = path.join(binDir, `flamegate-binary${ext}`);

  if (!fs.existsSync(binDir)) {
    fs.mkdirSync(binDir, { recursive: true });
  }

  console.log(`[flamegate] Detected platform: ${os}-${arch}`);
  console.log(`[flamegate] Downloading ${assetName} from GitHub Releases...`);

  // 1. Try latest release
  const latestUrl = `https://github.com/${GITHUB_REPO}/releases/latest/download/${assetName}`;
  const nightlyUrl = `https://github.com/${GITHUB_REPO}/releases/download/nightly/${assetName}`;

  try {
    await downloadFile(latestUrl, targetBinPath);
  } catch (err) {
    console.warn(`[flamegate] Latest release not available (${err.message}). Trying nightly release...`);
    try {
      await downloadFile(nightlyUrl, targetBinPath);
    } catch (nightlyErr) {
      console.error(`[flamegate] Download failed: ${nightlyErr.message}`);
      process.exit(1);
    }
  }

  // Set executable permissions on unix
  if (os !== 'windows') {
    fs.chmodSync(targetBinPath, 0o755);
  }

  console.log(`[flamegate] Successfully installed binary to ${targetBinPath}`);
}

install().catch((err) => {
  console.error(`[flamegate] Installation error:`, err);
  process.exit(1);
});
