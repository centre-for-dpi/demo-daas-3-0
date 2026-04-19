// Chromium-driven language picker test: uses real headless Chromium, clicks
// the topbar language dropdown, verifies the form submits and the page
// re-renders with translated text. Covers the gap that i18n-test.mjs left —
// that test only hit /lang via fetch, never exercised the dropdown onchange.
//
// Usage: VERIFIABLY_URL=http://localhost:8089 node e2e/chromium-lang-test.mjs

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

async function run() {
  const browser = await puppeteer.launch({
    executablePath: CHROME, headless: 'new',
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  });
  const page = await browser.newPage();
  page.on('pageerror', (e) => log(false, 'uncaught JS error', e.message));

  try {
    await page.goto(BASE + '/', { waitUntil: 'networkidle0' });
    const enSub = await page.$eval('.subtitle', (e) => e.textContent);
    await expect(/A thin, backend-agnostic/.test(enSub), 'EN: subtitle is English', enSub.slice(0, 80));

    // Change the language via the real <select>. onchange="this.form.submit()"
    // fires a POST /lang and the browser navigates (303 → GET /).
    await page.waitForSelector('#lang-form select[name="lang"]', { timeout: 5000 });
    await Promise.all([
      page.waitForNavigation({ waitUntil: 'networkidle0', timeout: 10000 }),
      page.select('#lang-form select[name="lang"]', 'fr'),
    ]);

    const frSub = await page.$eval('.subtitle', (e) => e.textContent);
    await expect(
      /Choisissez|interface mince/.test(frSub),
      'FR: dropdown change triggered translate',
      frSub.slice(0, 120),
    );

    // Verify the dropdown now shows "FR" selected.
    const selected = await page.$eval('#lang-form select[name="lang"]', (e) => e.value);
    await expect(selected === 'fr', 'FR: dropdown reflects the new language', 'value=' + selected);

    // Flip to Spanish the same way.
    await Promise.all([
      page.waitForNavigation({ waitUntil: 'networkidle0', timeout: 10000 }),
      page.select('#lang-form select[name="lang"]', 'es'),
    ]);
    const esSub = await page.$eval('.subtitle', (e) => e.textContent);
    await expect(
      /su rol|Elija|verificable|credencial/i.test(esSub),
      'ES: dropdown change triggered translate',
      esSub.slice(0, 120),
    );

    // Back to English.
    await Promise.all([
      page.waitForNavigation({ waitUntil: 'networkidle0', timeout: 10000 }),
      page.select('#lang-form select[name="lang"]', 'en'),
    ]);
    const backSub = await page.$eval('.subtitle', (e) => e.textContent);
    await expect(
      /A thin, backend-agnostic/.test(backSub),
      'EN: switching back restores English',
      backSub.slice(0, 80),
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
