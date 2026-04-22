// Regression check for the bulk-result UI fixes:
//   1. QR modal loads a real PNG (not /qr?data= which returned 400).
//   2. "Copy link" works on a non-secure-context origin (navigator.clipboard
//      is undefined — falls back to execCommand).
//   3. Offer link is fully readable + selectable (textarea, not truncated div).
import puppeteer from 'puppeteer-core';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const __dir = path.dirname(fileURLToPath(import.meta.url));
const BASE = process.env.BASE || 'http://172.24.0.1:8080';
const CSV = path.resolve(__dir, '..', 'testdata', 'bulk-issuance', 'csv', 'mortgage-simple.csv');

const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'] });
const ctx = await br.createBrowserContext();
const p = await ctx.newPage();
await p.setViewport({ width: 1400, height: 1000 });

async function auth(role) {
  await p.goto(`${BASE}/`, { waitUntil: 'domcontentloaded' });
  await p.click(`button.role-card[value="${role}"]`);
  await p.waitForFunction(() => /\/auth/.test(location.pathname), { timeout: 15000 });
  await Promise.all([ p.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(()=>null),
    p.evaluate(() => [...document.querySelectorAll('button.provider-btn')].find(b => (b.getAttribute('hx-vals')||'').includes('keycloak'))?.click()) ]);
  await p.waitForSelector('input[name="username"]', { timeout: 20000 });
  await p.type('input[name="username"]', 'admin'); await p.type('input[name="password"]', 'admin');
  await Promise.all([ p.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(()=>null),
    p.click('input[type="submit"], button[type="submit"]') ]);
  await new Promise(r => setTimeout(r, 700));
}
await auth('issuer');
await p.goto(`${BASE}/issuer/dpg`, { waitUntil: 'domcontentloaded' });
await p.waitForSelector('.dpg-card');
await p.evaluate(() => [...document.querySelectorAll('.dpg-card')].find(x => x.dataset.vendor === 'Walt Community Stack')?.click());
try { await p.waitForFunction(() => !document.querySelector('#issuer-dpg-continue')?.classList.contains('btn-disabled'), { timeout: 8000 }); } catch {}
await p.evaluate(() => document.querySelector('#issuer-dpg-continue')?.click());
await p.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(()=>{});
await p.goto(`${BASE}/issuer/schema`, { waitUntil: 'domcontentloaded' });
await p.waitForSelector('.schema-card');
await p.evaluate(() => {
  const c = [...document.querySelectorAll('.schema-card')].find(x => x.dataset.name === 'Mortgage Eligibility');
  const chip = [...(c?.querySelectorAll('.chip.small') || [])].find(x => x.title === 'jwt_vc_json');
  (chip || c)?.click();
});
await p.waitForNetworkIdle({ idleTime: 500 }).catch(()=>{});
await p.goto(`${BASE}/issuer/mode`, { waitUntil: 'domcontentloaded' });
await p.evaluate(() => {
  document.querySelector('input[name="scale"][value="bulk"]').checked = true;
  document.querySelector('input[name="dest"][value="wallet"]').checked = true;
  document.getElementById('mode-form').submit();
});
await p.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(()=>{});
await p.goto(`${BASE}/issuer/issue`, { waitUntil: 'domcontentloaded' });
await p.waitForSelector('input[type=file][name=csv_file]');
await (await p.$('input[type=file][name=csv_file]')).uploadFile(CSV);
await p.evaluate(() => {
  const btn = document.querySelector('form[hx-post*="csv"] button[type=submit]');
  if (btn) btn.click();
});
await p.waitForNetworkIdle({ idleTime: 800, timeout: 25000 }).catch(()=>{});
await new Promise(r => setTimeout(r, 1000));

// --- Assert (1): link column is a textarea with the full URI, selectable.
const linkProbe = await p.evaluate(() => {
  const ta = document.querySelector('.bulk-result-table tbody tr:first-child textarea');
  if (!ta) return { ok: false };
  return { ok: true, value: ta.value, tag: ta.tagName, full: ta.value.length > 120 && ta.value.startsWith('openid-credential-offer://') };
});
console.log('[1] link column:', JSON.stringify(linkProbe));
if (!linkProbe.ok || !linkProbe.full) { await br.close(); process.exit(1); }

// --- Assert (2): clicking "Copy link" writes to the clipboard via the fallback.
await p.evaluate(() => { window._clipboardWrites = []; const orig = document.execCommand.bind(document);
  document.execCommand = (cmd, ...rest) => {
    if (cmd === 'copy') {
      const sel = document.activeElement?.value || '';
      window._clipboardWrites.push(sel);
    }
    return orig(cmd, ...rest);
  };
});
await p.evaluate(() => {
  document.querySelector('.bulk-result-table tbody tr:first-child button[onclick*="bulkCopy"]').click();
});
await new Promise(r => setTimeout(r, 400));
const copyProbe = await p.evaluate(() => ({
  writes: window._clipboardWrites,
  firstBtnText: document.querySelector('.bulk-result-table tbody tr:first-child button[onclick*="bulkCopy"]').textContent,
}));
console.log('[2] copy fallback:', JSON.stringify(copyProbe));
const copied = (copyProbe.writes || [])[0] || '';
if (!copied.startsWith('openid-credential-offer://')) { await br.close(); process.exit(1); }

// --- Assert (3): QR modal loads a real PNG.
await p.evaluate(() => document.querySelector('.bulk-result-table tbody tr:first-child button[onclick*="openBulkQR"]').click());
await new Promise(r => setTimeout(r, 500));
const qrProbe = await p.evaluate(async () => {
  const img = document.querySelector('.modal-mask img[alt="OID4VCI QR"]');
  if (!img) return { ok: false };
  // Wait for load
  if (!img.complete) await new Promise(r => img.onload = img.onerror = r);
  const src = img.getAttribute('src');
  const resp = await fetch(src, { method: 'HEAD' });
  return { ok: true, src, status: resp.status, type: resp.headers.get('content-type'), w: img.naturalWidth, h: img.naturalHeight };
});
console.log('[3] QR modal:', JSON.stringify(qrProbe));
if (!qrProbe.ok || qrProbe.status !== 200 || !qrProbe.type?.includes('png') || qrProbe.w < 200) { await br.close(); process.exit(1); }

await br.close();
console.log('[ok] all three bulk-UI regressions pass');
