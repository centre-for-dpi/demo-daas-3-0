// Reproduce the user's issue: issue Mortgage Eligibility in jwt_vc_json,
// claim it, then try to present — assert matchPD reports > 0 matches.
import puppeteer from 'puppeteer-core';
const BASE = process.env.BASE || 'http://172.24.0.1:8080';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox', '--disable-dev-shm-usage'] });
function log(...a) { console.log('[m]', ...a); }
async function auth(p, role) {
  await p.goto(`${BASE}/`, { waitUntil: 'domcontentloaded' });
  await p.click(`button.role-card[value="${role}"]`);
  await p.waitForFunction(() => /\/auth/.test(location.pathname), { timeout: 15000 });
  await Promise.all([
    p.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(()=>null),
    p.evaluate(() => [...document.querySelectorAll('button.provider-btn')].find(b => (b.getAttribute('hx-vals')||'').includes('keycloak'))?.click()),
  ]);
  await p.waitForSelector('input[name="username"]', { timeout: 20000 });
  await p.type('input[name="username"]', 'admin');
  await p.type('input[name="password"]', 'admin');
  await Promise.all([
    p.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(()=>null),
    p.click('input[type="submit"], button[type="submit"]'),
  ]);
  await new Promise(r => setTimeout(r, 700));
}
async function pickDPG(p, role, vendor) {
  await p.goto(`${BASE}/${role}/dpg`, { waitUntil: 'domcontentloaded' });
  await p.waitForSelector('.dpg-card', { timeout: 15000 });
  await p.evaluate((v) => { const c = [...document.querySelectorAll('.dpg-card')].find(x => x.dataset.vendor === v); c?.click(); }, vendor);
  try { await p.waitForFunction((r) => { const b = document.querySelector(`#${r}-dpg-continue`); return b && !b.classList.contains('btn-disabled'); }, { timeout: 10000 }, role); } catch {}
  await p.evaluate((r) => document.querySelector(`#${r}-dpg-continue`)?.click(), role);
  await p.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 700));
}
async function issueMortgage() {
  const ctx = await br.createBrowserContext();
  const p = await ctx.newPage();
  await p.setViewport({ width: 1400, height: 1000 });
  await auth(p, 'issuer');
  await pickDPG(p, 'issuer', 'Walt Community Stack');
  await p.goto(`${BASE}/issuer/schema`, { waitUntil: 'domcontentloaded' });
  await p.waitForSelector('.schema-card', { timeout: 20000 });
  await p.evaluate(() => {
    const c = [...document.querySelectorAll('.schema-card')].find(x => x.dataset.name === 'Mortgage Eligibility');
    const chip = [...(c?.querySelectorAll('.chip.small') || [])].find(x => x.title === 'jwt_vc_json');
    (chip || c)?.click();
  });
  await p.waitForNetworkIdle({ idleTime: 400 }).catch(()=>{});
  await p.goto(`${BASE}/issuer/mode`, { waitUntil: 'domcontentloaded' });
  await p.evaluate(() => {
    document.querySelector('input[name="scale"][value="single"]').checked = true;
    document.querySelector('input[name="dest"][value="wallet"]').checked = true;
    document.getElementById('mode-form').submit();
  });
  await p.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(()=>{});
  await new Promise(r => setTimeout(r, 500));
  await p.goto(`${BASE}/issuer/issue`, { waitUntil: 'domcontentloaded' });
  await p.evaluate(() => {
    const i = document.querySelector('input[name="field_holder"]');
    if (i) { i.value = 'SmokeTestHolder'; i.dispatchEvent(new Event('input', { bubbles: true })); }
  });
  await p.evaluate(() => [...document.querySelectorAll('button')].find(x => /Issue credential/i.test(x.textContent||''))?.click());
  await p.waitForNetworkIdle({ idleTime: 500 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 1500));
  const offer = await p.evaluate(() => document.querySelector('.link-display')?.textContent?.trim());
  log('offer:', offer?.slice(0, 80) + '...');
  await p.close(); await ctx.close();
  return offer;
}
async function claim(offer) {
  const ctx = await br.createBrowserContext();
  const p = await ctx.newPage();
  await p.setViewport({ width: 1400, height: 1000 });
  await auth(p, 'holder');
  await pickDPG(p, 'holder', 'Walt Community Stack');
  await p.goto(`${BASE}/holder/wallet`, { waitUntil: 'domcontentloaded' });
  await p.evaluate((u) => {
    const ta = document.getElementById('offer-paste');
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
    setter.call(ta, u); ta.dispatchEvent(new Event('input', { bubbles: true })); ta.closest('form').requestSubmit();
  }, offer);
  await p.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 700));
  await p.evaluate(() => [...document.querySelectorAll('button')].find(x => /Accept/i.test((x.textContent||'').trim()))?.click());
  await p.waitForNetworkIdle({ idleTime: 600, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 900));
  return { ctx, p };
}
async function generateVerifierURI() {
  const ctx = await br.createBrowserContext();
  const vp = await ctx.newPage();
  await vp.setViewport({ width: 1400, height: 1000 });
  await auth(vp, 'verifier');
  await pickDPG(vp, 'verifier', 'Walt Community Stack');
  await vp.goto(`${BASE}/verifier/verify`, { waitUntil: 'domcontentloaded' });
  await vp.waitForSelector('#verifier-custom-body .schema-card', { timeout: 15000 });
  await vp.evaluate(() => {
    const c = [...document.querySelectorAll('#verifier-custom-body .schema-card')].find(x => x.dataset.name === 'Mortgage Eligibility');
    const chip = [...(c?.querySelectorAll('.chip.small') || [])].find(x => x.title === 'jwt_vc_json');
    (chip || c)?.click();
  });
  await vp.waitForNetworkIdle({ idleTime: 400 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 400));
  await vp.evaluate(() => {
    const b = [...document.querySelectorAll('#custom-template-form button[type="submit"]')].find(x => /Generate/i.test(x.textContent||''));
    b?.click();
  });
  await vp.waitForNetworkIdle({ idleTime: 500 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 700));
  const uri = await vp.evaluate(() => document.querySelector('#oid4vp-output .link-display')?.textContent?.trim());
  log('verifier URI:', uri?.slice(0, 80) + '...');
  await vp.close(); await ctx.close();
  return uri;
}
async function tryPresent(ctx, vURI) {
  const p = await ctx.newPage();
  await p.setViewport({ width: 1400, height: 1000 });
  await p.goto(`${BASE}/holder/present`, { waitUntil: 'domcontentloaded' });
  await p.evaluate((u) => {
    const ta = document.querySelector('textarea[name="request_uri"]');
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
    setter.call(ta, u); ta.dispatchEvent(new Event('input', { bubbles: true }));
  }, vURI);
  await p.evaluate(() => {
    const sel = document.querySelector('select[name="credential_id"]');
    const m = [...sel.options].reverse().find(o => /Mortgage/i.test(o.textContent) && o.dataset.format === 'jwt_vc_json');
    if (m) sel.value = m.value;
  });
  await p.evaluate(() => [...document.querySelectorAll('button[type="submit"]')].find(x => /Review/i.test(x.textContent||''))?.click());
  await p.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 700));
  const incompat = await p.evaluate(() => ({
    reason: document.querySelector('.present-consent')?.innerText?.slice(0, 500),
    blocked: !!document.querySelector('.present-consent.incompatible'),
  }));
  log('present preview:', JSON.stringify(incompat));
  return incompat;
}
try {
  const offer = await issueMortgage();
  const { ctx: holderCtx } = await claim(offer);
  const vURI = await generateVerifierURI();
  const preview = await tryPresent(holderCtx, vURI);
  await holderCtx.close();
  if (preview.blocked) { console.log('[m] FAIL: present was blocked'); process.exit(1); }
  console.log('[m] OK');
} catch (e) { console.error('[m] ERR:', e.message); process.exit(1); }
finally { await br.close(); }
