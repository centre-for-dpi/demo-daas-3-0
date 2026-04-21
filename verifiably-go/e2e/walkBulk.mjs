// Puppeteer test for the bulk-issuance source picker.
// Exercises all three sources in sequence:
//   1. CSV upload — uploads an in-memory CSV via the file input.
//   2. Secured API — points the UI at a host-side dummy server serving JSON rows.
//   3. Database — points the UI at a seeded postgres container on waltid_default.
//
// Pre-reqs (expected to be running):
//   * verifiably-go compose stack (./deploy.sh up all)
//   * Dummy API on the host at :9101 (see /tmp/bulk-test-api/main.go)
//   * Postgres container named `bulk-test-pg` on waltid_default network, with a
//     `graduates(holder, achievement, "issuedOn")` table containing rows.
//
// For each source the test asserts that the bulk-preview fragment renders and
// that the Accepted count is > 0.
import puppeteer from 'puppeteer-core';
import fs from 'fs';

const BASE = process.env.BASE || 'http://172.24.0.1:8080';
const br = await puppeteer.launch({
  executablePath: '/usr/bin/google-chrome',
  headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});

function log(...args) { console.log('[bulk]', ...args); }

async function auth(page, role) {
  await page.goto(`${BASE}/`, { waitUntil: 'domcontentloaded' });
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
  await page.waitForSelector('input[name="username"]', { timeout: 20000 });
  await page.type('input[name="username"]', 'admin');
  await page.type('input[name="password"]', 'admin');
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(() => null),
    page.click('input[type="submit"], button[type="submit"]'),
  ]);
  await new Promise(r => setTimeout(r, 700));
}

async function pickDPG(page, role, vendor) {
  await page.goto(`${BASE}/${role}/dpg`, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('.dpg-card', { timeout: 15000 });
  await page.evaluate((v) => {
    const c = [...document.querySelectorAll('.dpg-card')].find(x => x.dataset.vendor === v);
    c?.click();
  }, vendor);
  try {
    await page.waitForFunction(
      (r) => { const b = document.querySelector(`#${r}-dpg-continue`); return b && !b.classList.contains('btn-disabled'); },
      { timeout: 10000 }, role);
  } catch {}
  await page.evaluate((r) => document.querySelector(`#${r}-dpg-continue`)?.click(), role);
  await page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 10000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 800));
}

async function pickSchemaAndBulkMode(p) {
  await p.goto(`${BASE}/issuer/schema`, { waitUntil: 'domcontentloaded' });
  await p.waitForSelector('.schema-card', { timeout: 20000 });
  // Pick Open Badge via its jwt_vc_json chip (works with walt.id).
  await p.evaluate(() => {
    const card = [...document.querySelectorAll('.schema-card')].find(x => x.dataset.name === 'Open Badge Credential');
    const chip = [...(card?.querySelectorAll('.chip.small') || [])].find(x => x.title === 'jwt_vc_json');
    (chip || card?.querySelector('[hx-post*="schema/select"]'))?.click();
  });
  await p.waitForNetworkIdle({ idleTime: 500, timeout: 6000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 700));

  // Pick bulk + wallet mode.
  await p.goto(`${BASE}/issuer/mode`, { waitUntil: 'domcontentloaded' });
  await p.evaluate(() => {
    const scale = document.querySelector('input[name="scale"][value="bulk"]');
    const dest = document.querySelector('input[name="dest"][value="wallet"]');
    if (scale) scale.checked = true;
    if (dest) dest.checked = true;
    document.getElementById('mode-form')?.submit();
  });
  await p.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 10000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 500));
  log('landed on', p.url());
}

async function pickBulkSource(p, source) {
  // Chips live in the bulk panel. Click the one whose hx-vals contains our source.
  const clicked = await p.evaluate((src) => {
    const btns = [...document.querySelectorAll('button.chip')];
    const b = btns.find(x => (x.getAttribute('hx-vals') || '').includes(`"source": "${src}"`));
    if (b) { b.click(); return true; }
    return false;
  }, source);
  if (!clicked) throw new Error(`bulk source chip "${source}" not found`);
  // The handler swaps #bulk-form-wrap. Wait for the form to reflect the new source.
  await p.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 500));
}

async function runCSV(p) {
  log('=== CSV ===');
  await pickBulkSource(p, 'csv');
  // Write a throwaway CSV file, upload it via the input.
  const csv = 'holder,achievement,issuedOn\nCSV Alice,Pass,2026-04-21\nCSV Bob,Distinction,2026-04-21\n';
  const tmpPath = '/tmp/walk-bulk.csv';
  fs.writeFileSync(tmpPath, csv);
  const input = await p.$('input[name="csv_file"]');
  if (!input) throw new Error('no csv_file input after picking csv source');
  await input.uploadFile(tmpPath);
  await p.evaluate(() => document.querySelector('#bulk-form button[type="submit"]')?.click());
  await p.waitForNetworkIdle({ idleTime: 800, timeout: 15000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 800));
  const preview = await readPreview(p);
  log('csv preview →', preview);
  return preview;
}

async function runAPI(p) {
  log('=== API ===');
  await pickBulkSource(p, 'api');
  await p.evaluate(() => {
    const url = document.querySelector('input[name="api_url"]');
    url.value = 'http://host.docker.internal:9101/citizens';
    url.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await p.evaluate(() => document.querySelector('#bulk-form button[type="submit"]')?.click());
  await p.waitForNetworkIdle({ idleTime: 800, timeout: 30000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 1500));
  const preview = await readPreview(p);
  log('api preview →', preview);
  return preview;
}

async function runDB(p) {
  log('=== DB ===');
  await pickBulkSource(p, 'db');
  await p.evaluate(() => {
    const conn = document.querySelector('input[name="db_conn"]');
    const q = document.querySelector('textarea[name="db_query"]');
    conn.value = 'postgres://test:test@bulk-test-pg:5432/graduates?sslmode=disable';
    conn.dispatchEvent(new Event('input', { bubbles: true }));
    q.value = 'SELECT holder, achievement, "issuedOn"::text AS "issuedOn" FROM graduates';
    q.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await p.evaluate(() => document.querySelector('#bulk-form button[type="submit"]')?.click());
  await p.waitForNetworkIdle({ idleTime: 800, timeout: 30000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 1500));
  const preview = await readPreview(p);
  log('db preview →', preview);
  return preview;
}

async function readPreview(p) {
  return await p.evaluate(() => {
    const preview = document.querySelector('#csv-preview');
    const text = preview?.innerText?.trim() || '';
    const mAcc = text.match(/(\d+)\s+ACCEPTED/i);
    const mRej = text.match(/(\d+)\s+REJECTED/i);
    const m = mAcc ? [null, mAcc[1], mRej ? mRej[1] : '0'] : null;
    const flash = document.querySelector('.flash, .error-message, .alert, .toast')?.textContent?.trim();
    return {
      hasPreview: !!preview && preview.children.length > 0,
      accepted: m ? Number(m[1]) : null,
      rejected: m ? Number(m[2]) : null,
      flash: flash || null,
      snippet: text.slice(0, 300),
    };
  });
}

try {
  const ctx = await br.createBrowserContext();
  const p = await ctx.newPage();
  await p.setViewport({ width: 1400, height: 1000 });
  await auth(p, 'issuer');
  await pickDPG(p, 'issuer', 'Walt Community Stack');
  await pickSchemaAndBulkMode(p);

  const csv = await runCSV(p);
  const api = await runAPI(p);
  const db  = await runDB(p);

  const results = { csv, api, db };
  console.log('[bulk] RESULTS:', JSON.stringify(results, null, 2));

  let ok = true;
  for (const [k, v] of Object.entries(results)) {
    if (!v.hasPreview || v.accepted === null || v.accepted < 1) {
      console.log(`[bulk] FAIL: ${k} — ${JSON.stringify(v)}`);
      ok = false;
    }
  }
  if (ok) {
    console.log('[bulk] SUCCESS — all three sources produced at least one accepted row.');
    process.exit(0);
  }
  process.exit(1);
} catch (e) {
  console.error('[bulk] ERROR:', e.message);
  process.exit(1);
} finally {
  await br.close();
}
