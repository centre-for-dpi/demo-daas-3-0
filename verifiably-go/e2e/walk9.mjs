// Finer probe: look for format toggle keywords (sdjwtietf / sdjwtw3c / jwtw3c / jwt_vc_json / vc+sd-jwt / mso_mdoc)
// anywhere in the wizard DOM + grep the bundled JS to see what format keys the UI actually uses.

import puppeteer from 'puppeteer-core';

const browser = await puppeteer.launch({
  executablePath: '/usr/bin/google-chrome', headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
const page = await browser.newPage();
await page.setViewport({ width: 1600, height: 1400 });

const jsUrls = new Set();
page.on('request', (req) => { if (/\.js($|\?)/.test(req.url()) && /portal\.walt\.id/.test(req.url())) jsUrls.add(req.url()); });

await page.goto('https://portal.walt.id/', { waitUntil: 'networkidle2' });
await new Promise((r) => setTimeout(r, 2500));

// Click IdentityCredential (has multiple formats)
await page.evaluate(() => {
  const h6 = [...document.querySelectorAll('h6')].find((e) => (e.textContent || '').trim() === 'IdentityCredential');
  const card = h6?.closest('div.drop-shadow-sm') || h6?.closest('div');
  card?.click();
});
await new Promise((r) => setTimeout(r, 500));

// Before Start: does the tile show a format picker?
const tileState = await page.evaluate(() => {
  const h6 = [...document.querySelectorAll('h6')].find((e) => (e.textContent || '').trim() === 'IdentityCredential');
  const card = h6?.closest('div.drop-shadow-sm') || h6?.closest('div');
  return card ? card.outerHTML.slice(0, 3000) : null;
});
console.log('---- IdentityCredential tile HTML (after click) ----');
console.log(tileState);

// Click Start
await page.evaluate(() => {
  const b = [...document.querySelectorAll('button')].find((e) => /^\s*start\s*$/i.test(e.textContent || ''));
  b?.click();
});
await page.waitForNetworkIdle({ idleTime: 1200, timeout: 10000 }).catch(() => {});
await new Promise((r) => setTimeout(r, 2500));
console.log('URL after Start:', page.url());

// Full HTML of the wizard — let's grep for format keys
const html = await page.content();
const kws = ['sdjwtietf', 'sdjwtw3c', 'jwtw3c', 'jwt_vc_json', 'vc+sd-jwt', 'dc+sd-jwt', 'mso_mdoc', 'SD-JWT', 'W3C'];
console.log('\n---- keyword counts in rendered HTML ----');
for (const k of kws) {
  const re = new RegExp(k.replace(/[.+]/g, (m) => '\\' + m), 'gi');
  const c = (html.match(re) || []).length;
  if (c) console.log(`  ${k}: ${c}`);
}

// Hunt for bundled JS that defines format keys
console.log('\n---- Bundled JS URLs (will grep next) ----');
for (const u of [...jsUrls].slice(0, 12)) console.log('  ', u);

// Download each JS and grep
const fetch = (await import('node:https')).get;
async function grab(u) {
  return new Promise((res) => {
    fetch(u, (r) => { const chunks = []; r.on('data', (c) => chunks.push(c)); r.on('end', () => res(Buffer.concat(chunks).toString('utf8'))); });
  });
}
const seen = new Set();
for (const u of jsUrls) {
  const body = await grab(u);
  for (const k of ['sdjwtietf', 'sdjwtw3c', 'jwtw3c', 'vc+sd-jwt', 'mso_mdoc', 'credentialFormat', 'selectedFormat']) {
    if (body.includes(k)) {
      const idx = body.indexOf(k);
      const ctx = body.slice(Math.max(0, idx - 80), idx + 240).replace(/\s+/g, ' ');
      const key = k + '@' + u.split('/').pop();
      if (!seen.has(key)) {
        seen.add(key);
        console.log(`\n[match] ${key}:`);
        console.log('  ', ctx);
      }
    }
  }
}

// Also: take a screenshot and save the full HTML for later inspection
await page.screenshot({ path: '/tmp/walk9-wizard.png', fullPage: true });
const fs = await import('node:fs/promises');
await fs.writeFile('/tmp/walk9-wizard.html', html);
console.log('\n(screenshot at /tmp/walk9-wizard.png, html at /tmp/walk9-wizard.html)');

await browser.close();
