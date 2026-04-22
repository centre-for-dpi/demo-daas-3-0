// Test the Inji Certify pre-auth → walt.id wallet claim flow through the
// new /inji-preauth-bridge. The bridge stands in as a conformant OID4VCI
// issuer so walt.id's wallet can claim a pre-auth VC it otherwise rejects.
import puppeteer from 'puppeteer-core';

const BASE = process.env.BASE || 'http://172.24.0.1:8080';
const br = await puppeteer.launch({
  executablePath: '/usr/bin/google-chrome',
  headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});

function log(...args) { console.log('[bridge-preauth]', ...args); }

async function auth(page, role) {
  await page.goto(`${BASE}/`, { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector(`button.role-card[value="${role}"]`, { timeout: 15000 });
  await page.click(`button.role-card[value="${role}"]`);
  await page.waitForFunction(() => /\/auth/.test(location.pathname), { timeout: 15000 });
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(() => null),
    page.evaluate(() =>
      [...document.querySelectorAll('button.provider-btn')]
        .find(b => (b.getAttribute('hx-vals') || '').includes('keycloak'))?.click()
    ),
  ]);
  await new Promise(r => setTimeout(r, 2000));
  await page.screenshot({ path: '/tmp/auth-stage.png', fullPage: true });
  log('auth URL →', page.url());
  await page.waitForSelector('input[name="username"]', { timeout: 20000 });
  await page.type('input[name="username"]', 'admin');
  await page.type('input[name="password"]', 'admin');
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(() => null),
    page.click('input[type="submit"], button[type="submit"]'),
  ]);
  await new Promise(r => setTimeout(r, 600));
}

async function pickDPG(page, role, vendor) {
  await page.goto(`${BASE}/${role}/dpg`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  await page.waitForSelector('.dpg-card', { timeout: 15000 });
  const vendors = await page.evaluate(() =>
    [...document.querySelectorAll('.dpg-card')].map(x => x.dataset.vendor)
  );
  console.log('[bridge-preauth] available DPGs:', vendors);
  const matched = await page.evaluate((vendor) => {
    const c = [...document.querySelectorAll('.dpg-card')].find(x => x.dataset.vendor === vendor);
    if (c) { c.click(); return true; }
    return false;
  }, vendor);
  console.log('[bridge-preauth] pickDPG match →', matched, 'for', vendor);
  // Give HTMX time to apply the toggle response + OOB-swap the continue
  // button before we try to click it. waitForFunction returns immediately
  // once the class flips — longer timeout than before in case the backend
  // is slow on cold cache.
  try {
    await page.waitForFunction((role) => {
      const b = document.querySelector(`#${role}-dpg-continue`);
      return b && !b.classList.contains('btn-disabled');
    }, { timeout: 10000 }, role);
  } catch (e) {
    console.log('[bridge-preauth] continue stayed disabled:', e.message);
  }
  await page.evaluate((role) => document.querySelector(`#${role}-dpg-continue`)?.click(), role);
  await page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 10000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 1200));
  console.log('[bridge-preauth] after pickDPG, URL →', page.url());
}

async function issuePreAuth() {
  const ctx = await br.createBrowserContext();
  const p = await ctx.newPage();
  await p.setViewport({ width: 1400, height: 1000 });
  p.on('console', m => console.log('[page]', m.text()));
  await auth(p, 'issuer');
  await pickDPG(p, 'issuer', 'Inji Certify · Pre-Auth');
  await p.goto(`${BASE}/issuer/schema`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  await p.waitForSelector('.schema-card', { timeout: 20000 });
  // Click a format chip (NOT the card itself) — that's what
  // hx-post="/issuer/schema/select" binds to, which commits the schema + variant
  // to session. Clicking the card just expands it.
  const picked = await p.evaluate(() => {
    const cards = [...document.querySelectorAll('.schema-card')];
    const first = cards[0];
    if (!first) return null;
    console.log('first card html head: ' + first.outerHTML.slice(0, 400));
    // Cards may be collapsed — expand first via the card's own click/expand button.
    const expand = first.querySelector('[hx-post*="schema/expand"]');
    if (expand) expand.click();
    return first.dataset.name || null;
  });
  await new Promise(r => setTimeout(r, 700));
  const committed = await p.evaluate(() => {
    const cards = [...document.querySelectorAll('.schema-card')];
    const first = cards[0];
    if (!first) return null;
    const selectBtn = first.querySelector('[hx-post*="schema/select"]');
    if (selectBtn) selectBtn.click();
    return !!selectBtn;
  });
  log('schema select button clicked →', committed);
  const chipLog = await p.evaluate(() => window._lastChipLog || null);
  log('picked schema →', picked);
  log('picked schema →', picked);
  await p.waitForNetworkIdle({ idleTime: 600, timeout: 5000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 600));
  await p.goto(`${BASE}/issuer/mode`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  await p.evaluate(() => {
    // Native form — check the single + wallet radios, then submit.
    const scale = document.querySelector('input[name="scale"][value="single"]');
    const dest = document.querySelector('input[name="dest"][value="wallet"]');
    if (scale) scale.checked = true;
    if (dest) dest.checked = true;
    document.getElementById('mode-form')?.submit();
  });
  await p.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 10000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 400));
  await p.goto(`${BASE}/issuer/issue`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  await new Promise(r => setTimeout(r, 800));
  await p.evaluate(() => {
    const fills = {
      fullName: 'Bridge Test',
      mobileNumber: '9999999999',
      dateOfBirth: '1990-01-01',
      gender: 'Male',
      state: 'TN',
      district: 'Chennai',
      villageOrTown: 'Anna',
      postalCode: '600001',
      landArea: '10',
      landOwnershipType: 'Owned',
      primaryCropType: 'Rice',
      secondaryCropType: 'Wheat',
      farmerID: 'BT123',
      holder: 'Bridge Test',
    };
    for (const [name, val] of Object.entries(fills)) {
      const i = document.querySelector(`input[name="field_${name}"]`);
      if (i) { i.value = val; i.dispatchEvent(new Event('input', { bubbles: true })); }
    }
    // Catch any unlisted required fields.
    for (const i of document.querySelectorAll('input[name^="field_"]')) {
      if (!i.value) { i.value = 'x'; i.dispatchEvent(new Event('input', { bubbles: true })); }
    }
  });
  await p.evaluate(() =>
    [...document.querySelectorAll('button')].find(x => /Issue credential/i.test(x.textContent || ''))?.click()
  );
  await p.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 1500));
  const offer = await p.evaluate(() => document.querySelector('.link-display')?.textContent?.trim());
  log('pre-auth offer →', offer?.slice(0, 120) + '...');
  if (!offer) {
    await p.screenshot({ path: '/tmp/issue-stage.png', fullPage: true });
    const snapshot = await p.evaluate(() => ({
      url: location.href,
      title: document.title,
      flash: document.querySelector('.flash, .error-message, .toast, .alert')?.textContent?.trim(),
      body: document.body?.innerText?.slice(0, 500),
    }));
    log('issue stage →', JSON.stringify(snapshot));
  }
  await p.close();
  await ctx.close();
  if (!offer) throw new Error('no offer URL generated');
  return offer;
}

async function claim(offer) {
  const ctx = await br.createBrowserContext();
  const p = await ctx.newPage();
  await p.setViewport({ width: 1400, height: 1000 });
  await auth(p, 'holder');
  await pickDPG(p, 'holder', 'Walt Community Stack');
  await p.goto(`${BASE}/holder/wallet`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  await p.evaluate((u) => {
    const ta = document.getElementById('offer-paste');
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
    setter.call(ta, u);
    ta.dispatchEvent(new Event('input', { bubbles: true }));
    ta.closest('form').requestSubmit();
  }, offer);
  await p.waitForNetworkIdle({ idleTime: 600, timeout: 15000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 900));
  const pasteFeedback = await p.evaluate(() => ({
    flash: document.querySelector('.flash, .error-message, .alert, .toast')?.textContent?.trim(),
    pending: document.querySelectorAll('[data-state="pending"], .pending').length,
    allButtons: [...document.querySelectorAll('button')].map(b => b.textContent.trim()).filter(x => x),
    bodyTail: document.body?.innerText?.slice(-800),
  }));
  log('paste feedback →', JSON.stringify(pasteFeedback).slice(0, 800));
  const pendingBefore = await p.evaluate(() => document.querySelectorAll('[data-state="pending"], .pending').length);
  const accepted = await p.evaluate(() => {
    const btn = [...document.querySelectorAll('button')].find(x => x.textContent.trim() === 'Accept');
    if (btn) { btn.click(); return true; }
    return false;
  });
  log('pending-before →', pendingBefore, 'accept-clicked →', accepted);
  await p.waitForNetworkIdle({ idleTime: 800, timeout: 20000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 2000));
  await p.goto(`${BASE}/holder/wallet`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  const summary = await p.evaluate(() => {
    const cards = [...document.querySelectorAll('.wallet-card')];
    const farmer = cards.find(c => /Farmer/i.test(c.textContent));
    return {
      cards: cards.length,
      hasFarmer: !!farmer,
      farmerText: farmer?.textContent?.replace(/\s+/g, ' ').slice(0, 200),
      lastError: document.querySelector('.flash, .error-message, .alert, .toast')?.textContent?.trim(),
    };
  });
  log('wallet after claim →', JSON.stringify(summary));
  await p.close();
  return summary;
}

try {
  const offer = await issuePreAuth();
  const result = await claim(offer);
  if (result.hasFarmer) {
    console.log('[bridge-preauth] SUCCESS: Farmer credential from Inji pre-auth landed in walt.id wallet via bridge');
    console.log('[bridge-preauth] farmer card text:', result.farmerText);
    process.exit(0);
  } else {
    console.log('[bridge-preauth] FAIL: Farmer credential not found in wallet');
    console.log('[bridge-preauth]', JSON.stringify(result));
    process.exit(1);
  }
} catch (e) {
  console.error('[bridge-preauth] FAILED:', e.message);
  process.exit(1);
} finally {
  await br.close();
}
