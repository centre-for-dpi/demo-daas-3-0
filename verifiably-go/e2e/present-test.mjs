// M7 headless test: verifies the holder's Present page wires through to
// the adapter's PresentCredential method. Runs in MOCK mode so the assertion
// is deterministic — the adapter always returns Success=true for the mock.
// Real walt.id / injiweb paths are covered by the full matrix in M11.
//
// Usage:
//   VERIFIABLY_ADAPTER=mock VERIFIABLY_ADDR=:8089 ./verifiably &
//   VERIFIABLY_URL=http://localhost:8089 node e2e/present-test.mjs

import puppeteer from 'puppeteer-core';

const BASE = process.env.VERIFIABLY_URL || 'http://localhost:8089';
const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';

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
    // Navigate to present page as holder.
    await page.goto(BASE + '/', { waitUntil: 'networkidle0' });
    await page.click('button.role-card[value="holder"]');
    await settle(page);
    await page.click('form[action="/auth"] button[type="submit"]');
    await settle(page);

    // Pick the first non-redirect holder DPG.
    const vendor = await page.evaluate(() => {
      const cards = Array.from(document.querySelectorAll('.dpg-card'));
      const inline = cards.find((c) => !/Redirected|redirect/i.test(c.textContent));
      return inline ? inline.dataset.vendor : cards[0]?.dataset.vendor;
    });
    await expect(!!vendor, 'inline-capable holder DPG present', vendor || 'none');
    await page.click(`.dpg-card[data-vendor="${vendor}"]`);
    await settle(page);
    await page.click('#holder-dpg-continue');
    await page.waitForFunction(() => /\/holder\/wallet/.test(location.pathname), { timeout: 10000 });
    await settle(page);

    await page.goto(BASE + '/holder/present', { waitUntil: 'networkidle0' });
    await settle(page);

    // Form must render with a request_uri textarea and a credential_id select
    // — the latter is enabled only when ListWalletCredentials returned >=1.
    const hasForm = await page.$('form[hx-post="/holder/present/submit"]');
    await expect(!!hasForm, 'present form renders', '');

    const optCount = await page.$$eval('select[name="credential_id"] option', (els) => els.length);
    await expect(optCount > 0, 'credential picker has at least one option', 'n=' + optCount);

    // Submit through PresentCredential.
    await page.type('textarea[name="request_uri"]', 'openid4vp://?client_id=test&request_uri=https://verifier.test/oid4vp/abc');
    await page.evaluate(() => {
      const form = document.querySelector('form[hx-post="/holder/present/submit"]');
      form?.querySelector('button[type="submit"]')?.click();
    });
    await page.waitForFunction(
      () => /(Presentation sent|Presentation failed)/.test(document.getElementById('present-result')?.innerText || ''),
      { timeout: 10000 },
    );
    const res = await page.evaluate(() => document.getElementById('present-result')?.innerText || '');
    await expect(/Presentation sent/.test(res), 'presentation reported sent', res.slice(0, 200));
    await expect(/OID4VP/.test(res), 'method label includes OID4VP', res.slice(0, 200));
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
