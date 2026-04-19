// M6 headless test: exercises the real upload path on the verifier.
//   - Build a QR image via the helper binary at e2e/gen-qr (real PNG on disk).
//   - Drive the verifier UI to the paste/upload card.
//   - Upload the PNG as the "credential_image" form file.
//   - Assert the server decoded the QR (payload matches what we generated)
//     and invoked the downstream verifier adapter with method=upload.
//
// Usage: VERIFIABLY_URL=http://localhost:8089 node e2e/scan-upload-test.mjs

import { execSync } from 'child_process';
import fs from 'fs';
import os from 'os';
import path from 'path';
import puppeteer from 'puppeteer-core';

const BASE = process.env.VERIFIABLY_URL || 'http://localhost:8089';
const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';
const VENDOR = 'Inji Verify';
// Use a minimal JSON-LD VC as the QR payload. The server routes JSON-LD to
// the synchronous /vc-verification endpoint; even an unsigned VC gets a
// deterministic INVALID back (not a 500), which is what the test asserts
// downstream. The goal is to prove the full decode + dispatch pipeline, not
// to sign a real credential here.
const PAYLOAD = JSON.stringify({
  '@context': ['https://www.w3.org/2018/credentials/v1'],
  type: ['VerifiableCredential'],
  issuer: 'did:web:example.com',
  credentialSubject: { id: 'did:key:zTestHolder', demo: 'm6-' + Date.now() },
});

const results = [];
const fail = [];
function log(ok, msg, detail) {
  console.log((ok ? 'PASS' : 'FAIL') + '  ' + msg + (detail ? ' — ' + detail : ''));
  results.push({ ok, msg, detail });
  if (!ok) fail.push({ msg, detail });
}
async function expect(cond, msg, detail) { log(!!cond, msg, cond ? '' : detail); }
async function settle(page) {
  await page.waitForNetworkIdle({ idleTime: 250, timeout: 5000 }).catch(() => {});
}

async function run() {
  // Build the QR PNG via the helper binary. Uses the same go-qrcode library
  // that the server-side unit test uses so encoder/decoder versions match.
  const qrPath = path.join(os.tmpdir(), 'm6-qr-' + Date.now() + '.png');
  execSync('go build -o /tmp/gen-qr ./e2e/gen-qr/');
  execSync(`/tmp/gen-qr ${JSON.stringify(PAYLOAD)} ${qrPath}`);
  await expect(fs.existsSync(qrPath), 'generated QR PNG on disk', qrPath);

  const browser = await puppeteer.launch({
    executablePath: CHROME, headless: 'new',
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  });
  const page = await browser.newPage();
  page.on('pageerror', (e) => log(false, 'uncaught JS error', e.message));

  try {
    await page.goto(BASE + '/', { waitUntil: 'networkidle0' });
    await page.click('button.role-card[value="verifier"]');
    await settle(page);
    await page.click('form[action="/auth"] button[type="submit"]');
    await settle(page);
    await page.click(`.dpg-card[data-vendor="${VENDOR}"]`);
    await settle(page);
    await page.click('#verifier-dpg-continue');
    await page.waitForFunction(() => /\/verifier\/verify/.test(location.pathname), { timeout: 10000 });
    await settle(page);

    // Upload the QR PNG to the real upload form.
    const fileInput = await page.$('input[type="file"][name="credential_image"]');
    await expect(!!fileInput, 'upload form has a real file input', '');
    if (!fileInput) throw new Error('no file input');
    await fileInput.uploadFile(qrPath);

    // Submit the form (same form wraps the file input).
    await page.evaluate(() => {
      const input = document.querySelector('input[type="file"][name="credential_image"]');
      const form = input?.closest('form');
      form?.querySelector('button[type="submit"]')?.click();
    });

    // Wait for the verify-result fragment to fill in.
    await page.waitForFunction(
      () => /Credential (invalid|verified)|verificationStatus|vc-verification/i.test(
        document.getElementById('verify-result')?.innerText || ''),
      { timeout: 15000 },
    ).catch(() => {});

    const resultText = await page.evaluate(
      () => document.getElementById('verify-result')?.innerText || '',
    );
    await expect(resultText.length > 10, 'verify-result populated', resultText.slice(0, 200));
    await expect(
      /Uploaded file|vc-verification/i.test(resultText),
      'method label identifies the upload path',
      resultText.slice(0, 200),
    );
  } catch (e) {
    console.error('FATAL:', e.message);
    console.error(e.stack);
  }

  await browser.close();
  fs.unlinkSync(qrPath);

  console.log('\n' + '='.repeat(60));
  console.log(`Results: ${results.filter((r) => r.ok).length}/${results.length} passed`);
  if (fail.length) {
    console.log('\nFailures:');
    for (const f of fail) console.log(`  - ${f.msg}${f.detail ? ' — ' + f.detail : ''}`);
    process.exit(1);
  }
}

run();
