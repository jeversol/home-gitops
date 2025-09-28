#!/usr/bin/env node
const fs = require('fs');
const https = require('https');
const crypto = require('crypto');

function base64url(obj) {
  const buf = Buffer.isBuffer(obj) ? obj : Buffer.from(JSON.stringify(obj));
  return buf
    .toString('base64')
    .replace(/=/g, '')
    .replace(/\+/g, '-')
    .replace(/\//g, '_');
}

function exitWithError(message, detail) {
  console.error(message);
  if (detail) {
    console.error(detail);
  }
  process.exit(1);
}

const appId = process.env.RENOVATE_GITHUB_APP_ID;
const installationId = process.env.RENOVATE_GITHUB_APP_INSTALLATION_ID;
const rawKey = process.env.RENOVATE_GITHUB_APP_PRIVATE_KEY;

if (!appId || !installationId || !rawKey) {
  exitWithError('Missing GitHub App credentials in environment');
}

const privateKey = rawKey.replace(/\r?\n/g, '\n');

const now = Math.floor(Date.now() / 1000);
const header = { alg: 'RS256', typ: 'JWT' };
const payload = { iat: now - 60, exp: now + 540, iss: appId };

const unsigned = `${base64url(header)}.${base64url(payload)}`;
const signer = crypto.createSign('RSA-SHA256');
signer.update(unsigned);
signer.end();
const signature = base64url(signer.sign(privateKey));
const jwt = `${unsigned}.${signature}`;

const options = {
  hostname: 'api.github.com',
  path: `/app/installations/${installationId}/access_tokens`,
  method: 'POST',
  headers: {
    Authorization: `Bearer ${jwt}`,
    Accept: 'application/vnd.github+json',
    'User-Agent': 'renovate-self-hosted'
  }
};

const req = https.request(options, res => {
  let body = '';
  res.on('data', chunk => { body += chunk; });
  res.on('end', () => {
    if (res.statusCode < 200 || res.statusCode >= 300) {
      exitWithError(`GitHub API responded with status ${res.statusCode}`, body);
    }
    try {
      const parsed = JSON.parse(body);
      if (!parsed.token) {
        exitWithError('GitHub response missing token', body);
      }
      fs.writeFileSync('/renovate-auth/token', parsed.token, { mode: 0o600 });
      const installId = parsed.id ? String(parsed.id) : '';
      fs.writeFileSync('/renovate-auth/installation-id', installId, { mode: 0o600 });
      process.exit(0);
    } catch (err) {
      exitWithError('Failed to parse GitHub response', err);
    }
  });
});

req.on('error', err => exitWithError('Request failed', err));
req.end();
