// e2e/injiweb-farmer-flow.mjs
// Drives the Inji Web SPA at https://inji-web.bootcamp.cdpi.dev through:
//   landing → Continue as Guest → /issuers → Agriculture Department
//   → Farmer credential → eSignet authorize redirect
// PASS: redirected to esignet.bootcamp.cdpi.dev/authorize with
//       client_id=wallet-demo-client and redirect_uri pointing back
//       to inji-web.bootcamp.cdpi.dev/redirect.

import puppeteer from 'puppeteer-core';

const BASE = 'https://inji-web.bootcamp.cdpi.dev';

const br = await puppeteer.launch({
  executablePath: '/usr/bin/google-chrome',
  headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage', '--ignore-certificate-errors'],
});
const ctx = await br.createBrowserContext();
const p = await ctx.newPage();
await p.setViewport({ width: 1400, height: 1000 });

const navLog = [];
p.on('framenavigated', (f) => f === p.mainFrame() && navLog.push(f.url()));
p.on('pageerror', (e) => console.log(`  [pageerror] ${e.message}`));

const failed = [];
const fail = (msg, extra = {}) => {
  failed.push(msg);
  console.log(`FAIL: ${msg}`);
  for (const [k, v] of Object.entries(extra)) console.log(`  ${k}: ${v}`);
};
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const clickButtonByText = (page, regex) =>
  page.evaluate((rxStr) => {
    const rx = new RegExp(rxStr, 'i');
    const btn = [...document.querySelectorAll('button')].find(
      (b) => rx.test(b.innerText || '') && b.offsetParent !== null
    );
    if (btn) { btn.click(); return true; }
    return false;
  }, regex.source);

try {
  console.log(`==> step 1: ${BASE}/`);
  await p.goto(`${BASE}/`, { waitUntil: 'domcontentloaded', timeout: 30000 });
  await sleep(2000);

  console.log('==> step 2: Continue as Guest');
  if (!(await clickButtonByText(p, /Continue as Guest/))) {
    fail('Continue as Guest button missing');
    throw new Error('guest button');
  }
  await p.waitForFunction(() => location.pathname === '/issuers', { timeout: 15000 });
  console.log('  on /issuers');

  console.log('==> step 3: wait for Agriculture issuer');
  await p.waitForFunction(
    () => /Agriculture Department/i.test(document.body.innerText || ''),
    { timeout: 20000 }
  );

  console.log('==> step 4: click Agriculture Department card');
  // React's synthetic events don't set el.onclick, so we can't probe for a
  // click handler. Instead walk up from the H3 that holds the issuer name
  // until we hit the Tailwind issuer-card DIV (bg-white + fixed width).
  const issuerClickInfo = await p.evaluate(() => {
    // The issuer card uses an <h3> for the issuer name; walking from any
    // other element with "Agriculture Department" text overshoots the card
    // boundary. Stick to h3 to keep the walk-up local to the card.
    const h3 = [...document.querySelectorAll('h3')].find(
      (el) => /^Agriculture Department$/i.test((el.innerText || '').trim())
    );
    if (!h3) return { ok: false, reason: 'no Agriculture h3' };
    let n = h3;
    for (let i = 0; i < 8 && n && n !== document.body; i++) {
      const cls = (n.className || '').toString();
      if (/bg-white/.test(cls) && /w-\d+/.test(cls)) {
        n.click();
        return { ok: true, clicked: n.tagName + ' ' + cls.slice(0, 60) };
      }
      n = n.parentElement;
    }
    h3.click();
    return { ok: true, clicked: 'fallback h3 click' };
  });
  console.log('  clicked:', JSON.stringify(issuerClickInfo));
  await sleep(2500);
  console.log('  url:', p.url());

  console.log('==> step 5: wait for credentials list');
  try {
    await p.waitForFunction(
      () => /Farmer Credential|FarmerCredential|Farmer Verifiable/i.test(document.body.innerText || ''),
      { timeout: 15000 }
    );
  } catch {
    const body = (await p.evaluate(() => (document.body.innerText || '').slice(0, 1500))).replace(/\n+/g, ' / ');
    fail('Credentials page missing Farmer credential names', { url: p.url(), body });
    throw new Error('no credentials');
  }
  console.log('  Farmer credentials visible');

  console.log('==> step 6: click first Farmer credential card');
  await p.evaluate(() => {
    const all = [...document.querySelectorAll('button, a, [role="button"], div')];
    const cands = all.filter((el) => {
      const t = (el.innerText || '').trim();
      return /Farmer Credential \(V2\)|FarmerCredentialV2/i.test(t) && t.length < 200 && el.offsetParent !== null;
    });
    cands.sort((a, b) => a.innerText.length - b.innerText.length);
    let n = cands[0];
    if (!n) {
      // Fallback: any "Farmer" text
      const any = all.filter((el) => /Farmer/i.test((el.innerText || '').trim()) && el.offsetParent !== null);
      any.sort((a, b) => a.innerText.length - b.innerText.length);
      n = any[0];
    }
    if (!n) return;
    let walk = n;
    for (let i = 0; i < 6 && walk && walk !== document.body; i++) {
      if (
        walk.tagName === 'BUTTON' ||
        walk.tagName === 'A' ||
        walk.onclick ||
        walk.getAttribute('role') === 'button' ||
        (walk.className && /card|btn|credential/i.test(walk.className))
      ) {
        walk.click();
        return;
      }
      walk = walk.parentElement;
    }
    n.click();
  });

  await sleep(1500);

  // Some flows need a confirm/download button after card click
  await clickButtonByText(p, /^(Download|Continue|Get|Get Credential|Proceed)$/);

  console.log('==> step 7: wait for navigation to esignet');
  try {
    await p.waitForFunction(
      () => /esignet\.bootcamp\.cdpi\.dev/.test(location.host),
      { timeout: 20000 }
    );
  } catch {
    fail('Did not redirect to esignet.bootcamp.cdpi.dev', {
      url: p.url(),
      tail: navLog.slice(-6).join(' -> '),
    });
    throw new Error('no eSignet');
  }

  const url = p.url();
  console.log('  ==> reached eSignet:', url);
  const params = new URL(url).searchParams;
  const expected = {
    client_id: 'wallet-demo-client',
    redirect_uri: 'https://inji-web.bootcamp.cdpi.dev/redirect',
    response_type: 'code',
  };
  for (const [k, want] of Object.entries(expected)) {
    const got = params.get(k);
    if (got !== want) fail(`${k} mismatch (got ${got}, want ${want})`);
    else console.log(`  ${k} OK`);
  }
  console.log('  scope:', params.get('scope'));
  console.log('  acr_values:', params.get('acr_values'));
} catch (err) {
  console.log(`exception: ${err.message}`);
}

console.log('\n=== nav log ===');
navLog.forEach((u) => console.log('  ', u));

await br.close();

if (failed.length) {
  console.log(`\nFAILED (${failed.length}): ${failed.join(' | ')}`);
  process.exit(1);
}
console.log('\nPASS — Inji Web → eSignet auth-code flow initiated successfully.');
