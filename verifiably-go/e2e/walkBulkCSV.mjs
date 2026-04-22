// Issuer → pick Walt Community Stack → pick MortgageEligibility (1-field) →
// mode=bulk/csv → upload mortgage-simple.csv → verify "Accepted: 10" is rendered.
import puppeteer from 'puppeteer-core';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dir = path.dirname(fileURLToPath(import.meta.url));
const CSV = path.resolve(__dir, '..', 'testdata', 'bulk-issuance', 'csv', process.env.CSV || 'mortgage-simple.csv');
const BASE = process.env.BASE || 'http://172.24.0.1:8080';

const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'] });
const p = await (await br.createBrowserContext()).newPage();
await p.setViewport({ width: 1400, height: 1000 });
const log = (...a) => console.log('[b]', ...a);

async function auth(role) {
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
await auth('issuer');

// pick walt dpg
await p.goto(`${BASE}/issuer/dpg`, { waitUntil: 'domcontentloaded' });
await p.waitForSelector('.dpg-card', { timeout: 15000 });
await p.evaluate(() => [...document.querySelectorAll('.dpg-card')].find(x => x.dataset.vendor === 'Walt Community Stack')?.click());
try { await p.waitForFunction(() => { const b = document.querySelector('#issuer-dpg-continue'); return b && !b.classList.contains('btn-disabled'); }, { timeout: 10000 }); } catch {}
await p.evaluate(() => document.querySelector('#issuer-dpg-continue')?.click());
await p.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 10000 }).catch(()=>{});

// pick mortgage eligibility jwt_vc_json
await p.goto(`${BASE}/issuer/schema`, { waitUntil: 'domcontentloaded' });
await p.waitForSelector('.schema-card', { timeout: 20000 });
await p.evaluate(() => {
  const c = [...document.querySelectorAll('.schema-card')].find(x => x.dataset.name === 'Mortgage Eligibility');
  const chip = [...(c?.querySelectorAll('.chip.small') || [])].find(x => x.title === 'jwt_vc_json');
  (chip || c)?.click();
});
await p.waitForNetworkIdle({ idleTime: 500 }).catch(()=>{});

// mode = bulk + csv
await p.goto(`${BASE}/issuer/mode`, { waitUntil: 'domcontentloaded' });
await p.evaluate(() => {
  document.querySelector('input[name="scale"][value="bulk"]').checked = true;
  document.querySelector('input[name="dest"][value="wallet"]').checked = true;
  document.getElementById('mode-form').submit();
});
await p.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(()=>{});

// upload CSV
await p.goto(`${BASE}/issuer/issue`, { waitUntil: 'domcontentloaded' });
await p.waitForSelector('input[type=file][name=csv_file]', { timeout: 10000 });
const fileInput = await p.$('input[type=file][name=csv_file]');
await fileInput.uploadFile(CSV);
log('uploaded', path.basename(CSV));
await p.evaluate(() => document.querySelector('form.bulk-csv-form button[type=submit], form[hx-post*="csv"] button[type=submit]')?.click() || [...document.querySelectorAll('button')].find(x => /Issue bulk|Process|Simulate|Run/i.test(x.textContent||''))?.click());
await p.waitForNetworkIdle({ idleTime: 800, timeout: 20000 }).catch(()=>{});
await new Promise(r => setTimeout(r, 1000));

const summary = await p.evaluate(() => {
  const body = document.body.innerText;
  const m = body.match(/Accepted[^\d]*(\d+)[\s\S]{0,200}Rejected[^\d]*(\d+)/i);
  return { preview: body.slice(0, 600), accepted: m ? +m[1] : null, rejected: m ? +m[2] : null };
});
log('result', JSON.stringify({accepted:summary.accepted, rejected:summary.rejected}));
if (summary.accepted === null) log('preview:', summary.preview);
await br.close();
if (summary.accepted !== 10 || summary.rejected !== 0) process.exit(1);
console.log('[b] OK — 10 accepted / 0 rejected');
