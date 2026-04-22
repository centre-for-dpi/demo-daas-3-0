// Smoke test against the EC2 deploy. Exercises the three roles end-to-end:
//   issuer → schema pick → issue → offer URL
//   holder → paste offer → accept
//   verifier → custom request → present → check verdict
//
// Usage:  BASE=http://ec2-3-108-213-127.ap-south-1.compute.amazonaws.com:8080 node ec2-smoke.mjs
import puppeteer from 'puppeteer-core';

const BASE = process.env.BASE || 'http://ec2-3-108-213-127.ap-south-1.compute.amazonaws.com:8080';
const br = await puppeteer.launch({
  executablePath: '/usr/bin/google-chrome',
  headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});

function log(...args) { console.log('[smoke]', ...args); }

async function auth(page, role) {
  log(`auth(${role}) → landing`);
  await page.goto(`${BASE}/`, { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector(`button.role-card[value="${role}"]`, { timeout: 15000 });
  await page.click(`button.role-card[value="${role}"]`);
  await page.waitForFunction(() => /\/auth/.test(location.pathname), { timeout: 15000 });
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(() => null),
    page.evaluate(() =>
      [...document.querySelectorAll('button.provider-btn')]
        .find(b => (b.getAttribute('hx-vals') || '').includes('keycloak'))
        ?.click()
    ),
  ]);
  await page.waitForSelector('input[name="username"], input[name="password"]', { timeout: 20000 });
  await page.type('input[name="username"]', 'admin');
  await page.type('input[name="password"]', 'admin');
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 25000 }).catch(() => null),
    page.click('input[type="submit"], button[type="submit"]'),
  ]);
  await new Promise(r => setTimeout(r, 800));
  const url = page.url();
  log(`auth(${role}) → ${url}`);
  if (!url.includes(BASE.replace(/^https?:\/\//, '')) || url.includes('/auth')) {
    throw new Error(`auth(${role}) did not land back on app; got ${url}`);
  }
}

async function pickDPG(page, role, vendor) {
  log(`dpg(${role}) → ${vendor}`);
  await page.goto(`${BASE}/${role}/dpg`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  await page.waitForSelector('.dpg-card', { timeout: 15000 });
  const clicked = await page.evaluate((vendor) => {
    const c = [...document.querySelectorAll('.dpg-card')].find(x => x.dataset.vendor === vendor);
    if (!c) return false;
    c.click();
    return true;
  }, vendor);
  if (!clicked) throw new Error(`DPG card "${vendor}" not found on /${role}/dpg`);
  // Wait for the toggle to remove btn-disabled from the continue button.
  try {
    await page.waitForFunction(
      (role) => {
        const b = document.querySelector(`#${role}-dpg-continue`);
        return b && !b.classList.contains('btn-disabled');
      },
      { timeout: 5000 },
      role,
    );
  } catch {
    log(`dpg(${role}) continue stayed disabled — continuing anyway`);
  }
  await page.evaluate((role) => document.querySelector(`#${role}-dpg-continue`)?.click(), role);
  await page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 10000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 800));
}

async function issueWalletOffer() {
  log('=== issue ===');
  const ctx = await br.createBrowserContext();
  const p = await ctx.newPage();
  await p.setViewport({ width: 1400, height: 1000 });
  await auth(p, 'issuer');
  await pickDPG(p, 'issuer', 'Walt Community Stack');
  await p.goto(`${BASE}/issuer/schema`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  log('issuer/schema URL →', p.url());
  const schemaState = await p.evaluate(() => ({
    cards: document.querySelectorAll('.schema-card').length,
    bodySnippet: document.body?.innerText?.slice(0, 300),
    flashMsg: document.querySelector('.flash, .error-message, .alert')?.textContent?.trim(),
  }));
  log('issuer/schema state →', JSON.stringify(schemaState));
  if (schemaState.cards === 0) {
    await p.screenshot({ path: '/tmp/issuer-schema.png', fullPage: true });
    log('screenshot → /tmp/issuer-schema.png');
  }
  await p.waitForSelector('.schema-card', { timeout: 20000 });
  const picked = await p.evaluate(() => {
    const c = [...document.querySelectorAll('.schema-card')].find(x => x.dataset.name === 'Open Badge Credential');
    const chip = [...(c?.querySelectorAll('.chip.small') || [])].find(x => x.title === 'jwt_vc_json');
    if (chip) { chip.click(); return 'chip'; }
    if (c) { c.click(); return 'card'; }
    return null;
  });
  log('schema pick →', picked);
  await p.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(() => {});
  await p.goto(`${BASE}/issuer/mode`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  await p.evaluate(() => document.querySelector('button[hx-vals*="single"]')?.click());
  await new Promise(r => setTimeout(r, 300));
  await p.evaluate(() => document.querySelector('button[hx-vals*="wallet"]')?.click());
  await new Promise(r => setTimeout(r, 400));
  await p.goto(`${BASE}/issuer/issue`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  await new Promise(r => setTimeout(r, 700));
  await p.evaluate(() => {
    for (const [n, v] of [['holder', 'EC2TestHolder'], ['achievement', 'Passed'], ['issuedOn', '2026-04-21']]) {
      const i = document.querySelector(`input[name="field_${n}"]`);
      if (i) { i.value = v; i.dispatchEvent(new Event('input', { bubbles: true })); }
    }
  });
  await p.evaluate(() =>
    [...document.querySelectorAll('button')].find(x => /Issue credential/i.test(x.textContent || ''))?.click()
  );
  await p.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 1200));
  const offer = await p.evaluate(() => document.querySelector('.link-display')?.textContent?.trim());
  log('offer →', offer?.slice(0, 80) + '...');
  await p.close();
  await ctx.close();
  if (!offer) throw new Error('no offer URL captured from issuer flow');
  return offer;
}

async function claim(offer) {
  log('=== claim ===');
  const ctx = await br.createBrowserContext();
  const p = await ctx.newPage();
  await p.setViewport({ width: 1400, height: 1000 });
  await auth(p, 'holder');
  await pickDPG(p, 'holder', 'Walt Community Stack');
  await p.goto(`${BASE}/holder/wallet`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  log('holder/wallet URL →', p.url());
  const found = await p.evaluate(() => ({
    hasOfferPaste: !!document.getElementById('offer-paste'),
    hasTextarea: !!document.querySelector('textarea'),
    title: document.title,
    bodySnippet: document.body?.innerText?.slice(0, 200),
  }));
  log('holder/wallet snapshot →', JSON.stringify(found));
  await p.evaluate((u) => {
    const ta = document.getElementById('offer-paste') || document.querySelector('textarea[name="offer_uri"]');
    if (!ta) throw new Error('no #offer-paste textarea on /holder/wallet');
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
    setter.call(ta, u);
    ta.dispatchEvent(new Event('input', { bubbles: true }));
    ta.closest('form').requestSubmit();
  }, offer);
  await p.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 700));
  const accepted = await p.evaluate(() => {
    const btn = [...document.querySelectorAll('button')].find(x => /Accept/i.test((x.textContent || '').trim()));
    if (btn) { btn.click(); return true; }
    return false;
  });
  log('accept-button-clicked →', accepted);
  await p.waitForNetworkIdle({ idleTime: 600, timeout: 10000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 900));
  // verify credential now in wallet
  await p.goto(`${BASE}/holder/wallet`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  await new Promise(r => setTimeout(r, 700));
  const count = await p.evaluate(() => document.querySelectorAll('.wallet-card').length);
  log('wallet cards after claim →', count);
  if (count < 1) throw new Error('no credential in wallet after claim');
  await p.close();
  return ctx;
}

async function verify(holderCtx) {
  log('=== verify ===');
  const vCtx = await br.createBrowserContext();
  const vPage = await vCtx.newPage();
  await vPage.setViewport({ width: 1500, height: 1100 });
  await auth(vPage, 'verifier');
  await pickDPG(vPage, 'verifier', 'Walt Community Stack');
  await vPage.goto(`${BASE}/verifier/verify`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  await vPage.waitForSelector('#verifier-custom-body .schema-card', { timeout: 15000 });
  await vPage.evaluate(() => {
    const c = [...document.querySelectorAll('#verifier-custom-body .schema-card')].find(x => x.dataset.name === 'Open Badge Credential');
    const chip = [...(c?.querySelectorAll('.chip.small') || [])].find(x => x.title === 'jwt_vc_json');
    (chip || c)?.click();
  });
  await vPage.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 400));
  await vPage.evaluate(() => {
    const b = [...document.querySelectorAll('#custom-template-form button[type="submit"]')].find(x => /Generate/i.test(x.textContent || ''));
    b?.click();
  });
  await vPage.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 900));
  const vURI = await vPage.evaluate(() => document.querySelector('#oid4vp-output .link-display')?.textContent?.trim());
  log('verifier URI →', vURI?.slice(0, 80) + '...');

  // Present
  const hPage = await holderCtx.newPage();
  await hPage.setViewport({ width: 1400, height: 1100 });
  await hPage.goto(`${BASE}/holder/present`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  await new Promise(r => setTimeout(r, 500));
  await hPage.evaluate((u) => {
    const ta = document.querySelector('textarea[name="request_uri"]');
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
    setter.call(ta, u);
    ta.dispatchEvent(new Event('input', { bubbles: true }));
  }, vURI);
  await hPage.evaluate(() => {
    const sel = document.querySelector('select[name="credential_id"]');
    const m = [...sel.options].reverse().find(o => /Open Badge/i.test(o.textContent));
    if (m) sel.value = m.value;
  });
  await hPage.evaluate(() => {
    [...document.querySelectorAll('button[type="submit"]')].find(x => /Review/i.test(x.textContent || ''))?.click();
  });
  await hPage.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 700));
  await hPage.evaluate(() => {
    [...document.querySelectorAll('.present-consent button[type="submit"]')].find(x => /Disclose/i.test(x.textContent || ''))?.click();
  });
  await hPage.waitForNetworkIdle({ idleTime: 600, timeout: 15000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 1500));
  log('disclosed');

  // Poll verifier verdict
  let verdict = null;
  for (let i = 0; i < 15; i++) {
    const st = await vPage.evaluate(() => {
      const flip = document.querySelector('.verify-flip');
      return flip ? (flip.classList.contains('valid') ? 'VALID' : flip.classList.contains('invalid') ? 'INVALID' : 'OTHER') : null;
    });
    if (st === 'VALID' || st === 'INVALID') { verdict = st; break; }
    await new Promise(r => setTimeout(r, 2000));
  }
  log('verifier verdict →', verdict);
  return verdict;
}

let summary = { issue: false, claim: false, verify: null };
try {
  const offer = await issueWalletOffer();
  summary.issue = true;
  const holderCtx = await claim(offer);
  summary.claim = true;
  summary.verify = await verify(holderCtx);
} catch (e) {
  console.error('[smoke] FAILED:', e.message);
  summary.error = e.message;
} finally {
  await br.close();
  console.log('[smoke] SUMMARY:', JSON.stringify(summary));
  process.exit(summary.verify === 'VALID' ? 0 : 1);
}
