// M5 headless test: Inji Web Wallet is a redirect holder DPG. Selecting it
// routes through redirect_notice; the notice includes a link out to the
// Inji Web SPA (port 3004), plus a capability-list explaining the
// limitation (no server-to-server read-back API).
//
// Usage: VERIFIABLY_URL=http://localhost:8089 node e2e/injiweb-test.mjs

import puppeteer from 'puppeteer-core';

const BASE = process.env.VERIFIABLY_URL || 'http://localhost:8089';
const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';
const VENDOR = 'Inji Web Wallet';

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
    await page.click('button.role-card[value="holder"]');
    await settle(page);
    await page.click('form[action="/auth"] button[type="submit"]');
    await settle(page);

    const vendors = await page.$$eval('.dpg-card', (els) => els.map((e) => e.dataset.vendor));
    await expect(vendors.includes(VENDOR), 'Inji Web card rendered as holder DPG', vendors.join(', '));

    // Expand + read capabilities
    await page.click(`.dpg-card[data-vendor="${VENDOR}"]`);
    await settle(page);
    const caps = await page.evaluate((vendor) => {
      const card = document.querySelector(`.dpg-card[data-vendor="${vendor}"]`);
      return Array.from(card.querySelectorAll('.capability-item')).map((e) => ({
        kind: e.dataset.kind, key: e.dataset.key,
      }));
    }, VENDOR);
    await expect(
      caps.some((c) => c.kind === 'flow' && c.key === 'browser_hosted'),
      'card declares browser_hosted flow',
      JSON.stringify(caps.slice(0, 3)),
    );
    await expect(
      caps.some((c) => c.kind === 'limitation' && c.key === 'no_readback'),
      'card declares no_readback limitation',
      JSON.stringify(caps),
    );

    // Commit — Redirect=true should land on redirect_notice (not /holder/wallet)
    await page.click('#holder-dpg-continue');
    await settle(page);
    await page.waitForFunction(() => /External redirect|Opening/.test(document.body.innerText), { timeout: 5000 }).catch(() => {});
    const bodyText = await page.evaluate(() => document.body.innerText);
    await expect(/External redirect|Opening/.test(bodyText), 'redirect notice page rendered', bodyText.slice(0, 160));

    // The notice must link out to the configured UIURL. The SPA's MIMOTO_URL
    // is injected as PUBLIC_HOST:3004 (172.24.0.1 per shared .env); the link
    // has to match that origin so the SPA's XHRs stay same-origin and the
    // "No Credentials found" CORS trap doesn't happen.
    const links = await page.$$eval('a', (els) => els.map((e) => e.href));
    await expect(
      links.some((h) => /:3004(\/|$)/.test(h)),
      'notice links to Inji Web UI on port 3004',
      links.filter((h) => /:3004/.test(h)).join(', '),
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
