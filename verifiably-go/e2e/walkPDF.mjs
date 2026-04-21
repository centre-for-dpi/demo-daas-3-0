// End-to-end test for direct-to-PDF issuance via Inji Certify Pre-Auth.
// Drives the issuer UI through:
//   auth → Inji Certify · Pre-Auth DPG → FarmerCredential schema →
//   mode=single + dest=pdf → fill fields → Issue → click "Download your credential"
// Asserts the downloaded blob is a real PDF (starts with %PDF-).
import puppeteer from 'puppeteer-core';
import fs from 'fs';
import path from 'path';

const BASE = process.env.BASE || 'http://172.24.0.1:8080';
const DOWNLOAD_DIR = '/tmp/walk-pdf-downloads';
fs.rmSync(DOWNLOAD_DIR, { recursive: true, force: true });
fs.mkdirSync(DOWNLOAD_DIR, { recursive: true });

const br = await puppeteer.launch({
  executablePath: '/usr/bin/google-chrome',
  headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});

function log(...args) { console.log('[pdf]', ...args); }

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

async function run() {
  const ctx = await br.createBrowserContext();
  const p = await ctx.newPage();
  await p.setViewport({ width: 1400, height: 1000 });

  // Puppeteer download capture via CDP.
  const cdp = await p.target().createCDPSession();
  await cdp.send('Page.setDownloadBehavior', {
    behavior: 'allow',
    downloadPath: DOWNLOAD_DIR,
  });

  await auth(p, 'issuer');
  await pickDPG(p, 'issuer', 'Inji Certify · Pre-Auth');

  // Pick a schema via format chip.
  await p.goto(`${BASE}/issuer/schema`, { waitUntil: 'domcontentloaded' });
  await p.waitForSelector('.schema-card', { timeout: 20000 });
  const picked = await p.evaluate(() => {
    const cards = [...document.querySelectorAll('.schema-card')];
    const first = cards[0];
    if (!first) return null;
    const selectBtn = first.querySelector('[hx-post*="schema/select"]');
    (selectBtn || first).click();
    return first.dataset.name || null;
  });
  log('schema →', picked);
  await p.waitForNetworkIdle({ idleTime: 500, timeout: 6000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 700));

  // Mode: scale=single, dest=pdf.
  await p.goto(`${BASE}/issuer/mode`, { waitUntil: 'domcontentloaded' });
  const pdfTileEnabled = await p.evaluate(() => {
    const pdf = document.querySelector('input[name="dest"][value="pdf"]');
    return !!(pdf && !pdf.disabled);
  });
  log('pdf tile enabled →', pdfTileEnabled);
  if (!pdfTileEnabled) { throw new Error('PDF destination tile is disabled for this DPG'); }
  await p.evaluate(() => {
    document.querySelector('input[name="scale"][value="single"]').checked = true;
    document.querySelector('input[name="dest"][value="pdf"]').checked = true;
    document.getElementById('mode-form').submit();
  });
  await p.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 10000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 500));

  // Fill required fields.
  log('filling fields on', p.url());
  await p.evaluate(() => {
    const fills = {
      fullName: 'PDF Test Holder',
      mobileNumber: '9998887777',
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
      farmerID: 'PDF001',
      holder: 'PDF Test Holder',
    };
    for (const [name, val] of Object.entries(fills)) {
      const i = document.querySelector(`input[name="field_${name}"]`);
      if (i) { i.value = val; i.dispatchEvent(new Event('input', { bubbles: true })); }
    }
    for (const i of document.querySelectorAll('input[name^="field_"]')) {
      if (!i.value) { i.value = 'x'; i.dispatchEvent(new Event('input', { bubbles: true })); }
    }
  });
  await p.evaluate(() =>
    [...document.querySelectorAll('button')].find(x => /Issue credential/i.test(x.textContent || ''))?.click()
  );
  await p.waitForNetworkIdle({ idleTime: 800, timeout: 30000 }).catch(() => {});
  await new Promise(r => setTimeout(r, 1500));

  const result = await p.evaluate(() => {
    const link = document.querySelector('a[href*="/issuer/issue/pdf/"]');
    const flash = document.querySelector('.flash, .error-message, .toast, .alert')?.textContent?.trim();
    return {
      href: link?.getAttribute('href'),
      resultSnippet: document.querySelector('#issue-result')?.innerText?.slice(0, 300),
      flash,
    };
  });
  log('issue result →', JSON.stringify(result));
  if (!result.href) throw new Error('no PDF download link rendered');

  // Click the link. Chrome will trigger a download.
  await Promise.all([
    new Promise((resolve) => {
      const interval = setInterval(() => {
        const files = fs.readdirSync(DOWNLOAD_DIR).filter(f => !f.endsWith('.crdownload'));
        if (files.length > 0) { clearInterval(interval); resolve(); }
      }, 250);
      setTimeout(() => { clearInterval(interval); resolve(); }, 20000);
    }),
    p.evaluate((href) => {
      const a = document.querySelector(`a[href="${href}"]`);
      a?.click();
    }, result.href),
  ]);

  const files = fs.readdirSync(DOWNLOAD_DIR);
  log('downloaded files →', files);
  if (files.length === 0) throw new Error('no file downloaded');
  const file = path.join(DOWNLOAD_DIR, files[0]);
  const head = fs.readFileSync(file).slice(0, 8).toString();
  log('first bytes →', JSON.stringify(head));
  const size = fs.statSync(file).size;
  log('size →', size, 'bytes');
  if (!head.startsWith('%PDF-')) throw new Error(`downloaded file is not a PDF (head=${head})`);
  if (size < 1000) throw new Error(`PDF suspiciously small: ${size} bytes`);

  console.log('[pdf] SUCCESS — downloaded a valid PDF of', size, 'bytes');
}

try {
  await run();
  process.exit(0);
} catch (e) {
  console.error('[pdf] FAILED:', e.message);
  process.exit(1);
} finally {
  await br.close();
}
