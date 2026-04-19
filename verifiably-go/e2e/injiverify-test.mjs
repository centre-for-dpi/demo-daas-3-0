// M4 headless test: exercises the Inji Verify card through the verifier UI.
// Verifies:
//   - Inji Verify renders as a verifier DPG with distinct capabilities.
//   - Direct paste verification hits /v1/verify/vc-verification on the live
//     service and surfaces the returned verificationStatus (INVALID on an
//     unsigned credential).
//   - INJIVER-1131 guard is declared on the card.
//
// Usage: VERIFIABLY_URL=http://localhost:8089 node e2e/injiverify-test.mjs

import puppeteer from 'puppeteer-core';

const BASE = process.env.VERIFIABLY_URL || 'http://localhost:8089';
const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';
const VENDOR = 'Inji Verify';

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

    const vendors = await page.$$eval('.dpg-card', (els) => els.map((e) => e.dataset.vendor));
    await expect(vendors.includes(VENDOR), 'Inji Verify card rendered', vendors.join(', '));

    // Expand and read capabilities
    await page.click(`.dpg-card[data-vendor="${VENDOR}"]`);
    await settle(page);
    const caps = await page.evaluate((vendor) => {
      const card = document.querySelector(`.dpg-card[data-vendor="${vendor}"]`);
      return Array.from(card.querySelectorAll('.capability-item')).map((e) => ({
        kind: e.dataset.kind, key: e.dataset.key,
      }));
    }, VENDOR);
    await expect(
      caps.some((c) => c.kind === 'flow' && c.key === 'direct'),
      'declares synchronous /vc-verification flow',
      JSON.stringify(caps.slice(0, 4)),
    );
    await expect(
      caps.some((c) => c.kind === 'limitation' && c.key === 'injiver_1131'),
      'declares INJIVER-1131 guard',
      JSON.stringify(caps),
    );

    // Commit DPG → navigate to /verifier/verify
    await page.click('#verifier-dpg-continue');
    await page.waitForFunction(() => /\/verifier\/verify/.test(location.pathname), { timeout: 10000 });
    await settle(page);

    // Direct verify with a paste of an unsigned JSON-LD VC — Inji Verify
    // should return INVALID (no proof). The UI renders "Credential invalid".
    const unsigned = JSON.stringify({
      '@context': ['https://www.w3.org/2018/credentials/v1'],
      type: ['VerifiableCredential'],
      issuer: 'did:web:example.com',
      credentialSubject: { id: 'did:key:z6MkTestHolder' },
    });
    await page.waitForSelector('textarea[name="credential_data"]', { timeout: 5000 });
    await page.evaluate((s) => {
      const el = document.querySelector('textarea[name="credential_data"]');
      if (el) el.value = s;
    }, unsigned);

    // Submit the paste form directly — it's the form whose hx-post is
    // /verifier/verify/direct AND contains a textarea (distinguishes it from
    // the upload form, which also hx-posts the same path).
    await page.evaluate(() => {
      const forms = Array.from(document.querySelectorAll('form'));
      const form = forms.find((f) =>
        f.getAttribute('hx-post') === '/verifier/verify/direct' &&
        f.querySelector('textarea[name="credential_data"]'),
      );
      if (form) {
        const btn = form.querySelector('button[type="submit"]');
        if (btn) btn.click();
      }
    });
    await page.waitForFunction(
      () => /(invalid|Credential invalid|INVALID)/i.test(document.body.innerText),
      { timeout: 10000 },
    ).catch(() => {});

    const body = await page.evaluate(() => document.body.innerText);
    await expect(/invalid/i.test(body), 'direct paste returns INVALID for unsigned VC', body.slice(0, 200));
    await expect(
      /vc-verification/i.test(body),
      'method label names the backend endpoint used',
      body.slice(0, 200),
    );
  } catch (e) {
    console.error('FATAL:', e.message);
    console.error(e.stack);
  }

  await browser.close();
  console.log('\n' + '='.repeat(60));
  console.log(`Results: ${results.filter((r) => r.ok).length}/${results.length} passed`);
  if (fail.length) {
    console.log('\nFailures:');
    for (const f of fail) console.log(`  - ${f.msg}${f.detail ? ' — ' + f.detail : ''}`);
    process.exit(1);
  }
}

run();
