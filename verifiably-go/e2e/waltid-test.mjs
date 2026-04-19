// M2 headless test: drives the issuer/holder/verifier flows end-to-end
// against a registry-mode verifiably-go backed by live walt.id v0.18.2
// (docker-compose ports 7001/7002/7003).
//
// Prereqs:
//   - walt.id compose services running on 7001/7002/7003
//   - verifiably-go running in registry mode:
//       VERIFIABLY_ADAPTER=registry VERIFIABLY_ADDR=:8089 ./verifiably
//   - VERIFIABLY_URL env var points at that server
//
// Usage: VERIFIABLY_URL=http://localhost:8089 node e2e/waltid-test.mjs

import puppeteer from 'puppeteer-core';

const BASE = process.env.VERIFIABLY_URL || 'http://localhost:8089';
const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';
const VENDOR = 'Walt Community Stack';

const results = [];
const fail = [];

function log(ok, msg, detail) {
  const line = (ok ? 'PASS' : 'FAIL') + '  ' + msg + (detail ? ' — ' + detail : '');
  console.log(line);
  results.push({ ok, msg, detail });
  if (!ok) fail.push({ msg, detail });
}
async function expect(cond, msg, detail) { log(!!cond, msg, cond ? '' : detail); }

async function settle(page) {
  await page.waitForNetworkIdle({ idleTime: 250, timeout: 4000 }).catch(() => {});
}

async function click(page, selector) {
  await page.waitForSelector(selector, { visible: true, timeout: 5000 });
  await page.$eval(selector, (el) => el.scrollIntoView({ block: 'center', behavior: 'instant' }));
  await page.click(selector);
}

async function run() {
  const browser = await puppeteer.launch({
    executablePath: CHROME,
    headless: 'new',
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  });
  const page = await browser.newPage();
  page.on('pageerror', (e) => log(false, 'uncaught JS error', e.message));

  try {
    // ====== ISSUER FLOW with LIVE WALT.ID ======
    await page.goto(BASE + '/', { waitUntil: 'networkidle0' });
    await click(page, 'button.role-card[value="issuer"]');
    await settle(page);
    await click(page, 'form[action="/auth"] button[type="submit"]');
    await settle(page);

    // DPG: expect exactly 1 (walt.id registered), card must carry the vendor.
    const issuerCards = await page.$$eval('.dpg-card', (els) => els.map((e) => e.dataset.vendor));
    await expect(issuerCards.includes(VENDOR), 'issuer DPG card present', `cards=${JSON.stringify(issuerCards)}`);
    await click(page, `.dpg-card[data-vendor="${VENDOR}"] .dpg-card-head`);
    await settle(page);
    await click(page, '#issuer-dpg-continue');
    await settle(page);

    // Schema browser: expect live walt.id credential configurations.
    const schemaCount = await page.$$eval('.schema-card', (els) => els.length);
    await expect(schemaCount > 100, 'schema count from live issuer', `got ${schemaCount}`);
    // Find UniversityDegree card and click its "Use this schema" button
    const pickedID = await page.evaluate(() => {
      const cards = Array.from(document.querySelectorAll('.schema-card'));
      const target = cards.find((c) => (c.dataset.id || '').startsWith('UniversityDegree_jwt_vc_json'));
      if (!target) return '';
      const btn = target.querySelector('button[hx-post="/issuer/schema/select"]');
      if (btn) btn.click();
      return target.dataset.id;
    });
    await expect(pickedID.startsWith('UniversityDegree_jwt_vc_json'), 'UniversityDegree config found', pickedID);
    await settle(page);
    // Navigate directly — session state (SchemaID) is already set by the click above.
    await page.goto(BASE + '/issuer/mode', { waitUntil: 'networkidle0' });
    await expect(await page.$('#mode-form'), 'mode page rendered', await page.title());

    // Mode → continue to issue form
    await Promise.all([
      page.waitForNavigation({ waitUntil: 'networkidle0', timeout: 6000 }),
      page.click('#mode-form button[type="submit"]'),
    ]);

    // Issue: fill and submit (the issue button uses hx-post, not a form submit)
    await page.waitForSelector('#single-form-el', { timeout: 6000 });
    await page.evaluate(() => {
      const holder = document.querySelector('input[name="field_holder"]');
      if (holder) holder.value = 'Achieng Otieno';
    });
    await page.click('button[hx-post="/issuer/issue"]');
    await page.waitForFunction(
      () => /openid-credential-offer:\/\//.test(document.body.innerText),
      { timeout: 15000 },
    );
    await settle(page);

    // The result fragment should render a real openid-credential-offer URI
    // whose target is walt.id's issuer-api on port 7002.
    const offerText = await page.evaluate(() => document.body.innerText);
    const offerMatch = offerText.match(/openid-credential-offer:\/\/[^\s"<]+/);
    await expect(!!offerMatch, 'real offer URI rendered', offerMatch ? offerMatch[0].slice(0, 80) : 'none');
    await expect(
      offerMatch && /localhost%3A7002|localhost:7002/.test(offerMatch[0]),
      'offer URI references live walt.id (port 7002)',
      offerMatch ? offerMatch[0] : '',
    );

    // Fetch the offer directly via HTTP to confirm walt.id signed a real pre-auth code.
    if (offerMatch) {
      const inner = decodeURIComponent(offerMatch[0].split('credential_offer_uri=')[1] || '');
      if (inner) {
        const resolved = await fetch(inner).then((r) => r.json()).catch(() => null);
        await expect(
          resolved && resolved.grants && Object.keys(resolved.grants).some((k) => k.includes('pre-authorized')),
          'credential offer carries real pre-auth grant',
          resolved ? JSON.stringify(Object.keys(resolved.grants || {})) : 'fetch failed',
        );
      }
    }

    // ====== HOLDER FLOW ======
    await page.goto(BASE + '/', { waitUntil: 'networkidle0' });
    await click(page, 'button.role-card[value="holder"]');
    await settle(page);
    await click(page, 'form[action="/auth"] button[type="submit"]');
    await settle(page);

    // Pick the holder DPG
    const holderCards = await page.$$eval('.dpg-card', (els) => els.map((e) => e.dataset.vendor));
    await expect(holderCards.includes(VENDOR), 'holder DPG present', `cards=${JSON.stringify(holderCards)}`);
    await click(page, `.dpg-card[data-vendor="${VENDOR}"] .dpg-card-head`);
    await settle(page);
    await click(page, '#holder-dpg-continue');
    await settle(page);

    // Wallet: paste offer URI to resolve it against live wallet-api.
    if (offerMatch) {
      await page.waitForSelector('textarea[name="offer_uri"]');
      await page.$eval('textarea[name="offer_uri"]', (el, val) => { el.value = val; }, offerMatch[0]);
      await page.click('form[hx-post="/holder/wallet/paste"] button[type="submit"]');
      await page.waitForFunction(
        () => /(pending|Incoming|offer)/.test(document.querySelector('#wallet-body')?.innerText || ''),
        { timeout: 15000 },
      );
      const walletBody = await page.$eval('#wallet-body', (el) => el.innerText).catch(() => '');
      await expect(/Incoming credential|pending|offer/i.test(walletBody), 'pending credential appears', walletBody.slice(0, 160));
    }

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
