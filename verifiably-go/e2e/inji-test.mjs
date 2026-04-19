// M3 headless test: verifies both Inji Certify cards render with distinct
// capability lists, produce different-shaped offers, and surface correctly
// through the UI. Runs against a registry-mode verifiably-go backed by:
//   - inji-certify (Auth-Code via eSignet) on port 8091
//   - inji-certify-preauth (Pre-Auth only) on port 8094
//
// Usage: VERIFIABLY_URL=http://localhost:8089 node e2e/inji-test.mjs

import puppeteer from 'puppeteer-core';

const BASE = process.env.VERIFIABLY_URL || 'http://localhost:8089';
const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';
const AUTHCODE = 'Inji Certify · Auth-Code';
const PREAUTH  = 'Inji Certify · Pre-Auth';

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

async function driveIssue(page, vendor, schemaID, fields) {
  // Fresh session per driveIssue — clear cookies so previous toggle state
  // doesn't flip the card off on the next click.
  const client = await page.target().createCDPSession();
  await client.send('Network.clearBrowserCookies').catch(() => {});
  await page.goto(BASE + '/', { waitUntil: 'networkidle0' });
  await page.click('button.role-card[value="issuer"]');
  await settle(page);
  await page.click('form[action="/auth"] button[type="submit"]');
  await settle(page);

  // Select the vendor's DPG card
  const present = await page.$(`.dpg-card[data-vendor="${vendor}"]`);
  if (!present) throw new Error(`card not present: ${vendor}`);
  await page.click(`.dpg-card[data-vendor="${vendor}"]`);
  await settle(page);
  // Continue button POSTs /issuer/dpg which returns HX-Redirect — HTMX then
  // navigates the full page. Click it and wait for URL change.
  await page.click('#issuer-dpg-continue');
  await page.waitForFunction(
    () => /\/issuer\/schema/.test(location.pathname),
    { timeout: 10000 },
  );
  await settle(page);

  // Select the given schema ID
  const selected = await page.evaluate((id) => {
    const cards = Array.from(document.querySelectorAll('.schema-card'));
    const target = cards.find((c) => (c.dataset.id || '') === id);
    if (!target) return '';
    const btn = target.querySelector('button[hx-post="/issuer/schema/select"]');
    if (btn) btn.click();
    return target.dataset.id;
  }, schemaID);
  if (selected !== schemaID) throw new Error(`schema ${schemaID} not found`);
  await settle(page);
  await page.goto(BASE + '/issuer/mode', { waitUntil: 'networkidle0' });
  // Mode form submit is a plain HTML form POST → 303 See Other → full nav.
  await page.click('#mode-form button[type="submit"]');
  await page.waitForSelector('#single-form-el', { timeout: 10000 });

  // Fill the form fields we were given
  await page.waitForSelector('#single-form-el', { timeout: 6000 });
  for (const [k, v] of Object.entries(fields)) {
    await page.evaluate((name, val) => {
      const el = document.querySelector(`input[name="${name}"]`);
      if (el) el.value = val;
    }, `field_${k}`, v);
  }
  await page.click('button[hx-post="/issuer/issue"]');
  await page.waitForFunction(
    () => /openid-credential-offer:\/\//.test(document.body.innerText),
    { timeout: 15000 },
  );
  await settle(page);

  const offerText = await page.evaluate(() => document.body.innerText);
  const match = offerText.match(/openid-credential-offer:\/\/[^\s"<]+/);
  return match ? match[0].replace(/&amp;/g, '&').replace(/&#43;/g, '+') : '';
}

async function fetchOffer(offerURI) {
  const inner = decodeURIComponent(offerURI.split('credential_offer_uri=')[1] || '');
  return fetch(inner).then((r) => r.json());
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
    // ====== DPG picker: both cards must be present ======
    await page.goto(BASE + '/', { waitUntil: 'networkidle0' });
    await page.click('button.role-card[value="issuer"]');
    await settle(page);
    await page.click('form[action="/auth"] button[type="submit"]');
    await settle(page);

    const vendors = await page.$$eval('.dpg-card', (els) => els.map((e) => e.dataset.vendor));
    await expect(vendors.includes(AUTHCODE), 'Auth-Code card rendered', vendors.join(', '));
    await expect(vendors.includes(PREAUTH),  'Pre-Auth card rendered',  vendors.join(', '));

    // Expand AUTHCODE and read its capability kinds
    await page.click(`.dpg-card[data-vendor="${AUTHCODE}"]`);
    await settle(page);
    const acCaps = await page.evaluate((vendor) => {
      const card = document.querySelector(`.dpg-card[data-vendor="${vendor}"]`);
      return Array.from(card.querySelectorAll('.capability-item')).map((e) => ({
        kind: e.dataset.kind, key: e.dataset.key,
      }));
    }, AUTHCODE);
    await expect(
      acCaps.some((c) => c.kind === 'flow' && c.key === 'auth_code'),
      'Auth-Code card surfaces auth_code flow capability',
      JSON.stringify(acCaps.slice(0, 3)),
    );
    await expect(
      acCaps.some((c) => c.kind === 'token' && c.key === 'idp_signed'),
      'Auth-Code card declares IdP-signed tokens',
      JSON.stringify(acCaps),
    );

    // Expand PREAUTH and read its capability kinds
    // Navigate back so Expanded resets, then expand PREAUTH.
    await page.goto(BASE + '/issuer/dpg', { waitUntil: 'networkidle0' });
    await page.click(`.dpg-card[data-vendor="${PREAUTH}"]`);
    await settle(page);
    const paCaps = await page.evaluate((vendor) => {
      const card = document.querySelector(`.dpg-card[data-vendor="${vendor}"]`);
      return Array.from(card.querySelectorAll('.capability-item')).map((e) => ({
        kind: e.dataset.kind, key: e.dataset.key,
      }));
    }, PREAUTH);
    await expect(
      paCaps.some((c) => c.kind === 'flow' && c.key === 'pre_auth'),
      'Pre-Auth card surfaces pre_auth flow capability',
      JSON.stringify(paCaps.slice(0, 3)),
    );
    await expect(
      paCaps.some((c) => c.kind === 'limitation' && c.key === 'not_inji_web'),
      'Pre-Auth card declares incompatibility with Inji Web',
      JSON.stringify(paCaps),
    );

    // ====== Pre-Auth card: full issuance end-to-end ======
    const farmerFields = {
      fullName: 'Achieng Otieno',
      mobileNumber: '0712345678',
      dateOfBirth: '1990-01-01',
      gender: 'Female',
      state: 'KE',
      district: 'Nairobi',
      villageOrTown: 'City',
      postalCode: '00100',
      landArea: '1',
      landOwnershipType: 'Owned',
    };
    const paOffer = await driveIssue(page, PREAUTH, 'FarmerCredentialV2', farmerFields);
    await expect(paOffer, 'Pre-Auth produced an offer URI', paOffer.slice(0, 80));
    await expect(/localhost%3A8094/.test(paOffer), 'Pre-Auth offer references public URL :8094', paOffer);
    const paData = await fetchOffer(paOffer);
    await expect(
      paData.grants && Object.keys(paData.grants).some((k) => k.includes('pre-authorized')),
      'Pre-Auth offer carries pre-authorized_code grant',
      JSON.stringify(paData.grants || {}),
    );

    // ====== Auth-Code card: issuance produces a hosted offer JSON ======
    const acOffer = await driveIssue(page, AUTHCODE, 'FarmerCredentialV2', farmerFields);
    await expect(acOffer, 'Auth-Code produced an offer URI', acOffer.slice(0, 80));
    await expect(/(\/offers\/|%2Foffers%2F)/.test(acOffer), 'Auth-Code offer is hosted by verifiably-go', acOffer);
    const acData = await fetchOffer(acOffer);
    await expect(
      acData.grants && 'authorization_code' in acData.grants,
      'Auth-Code offer carries authorization_code grant',
      JSON.stringify(acData.grants || {}),
    );
    await expect(
      acData.grants && acData.grants.authorization_code && acData.grants.authorization_code.authorization_server,
      'Auth-Code offer names an authorization_server',
      JSON.stringify(acData.grants?.authorization_code || {}),
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
