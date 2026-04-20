// Probe walt.id's own portal UX for the issue-a-credential flow:
//  1. what the credential cards look like
//  2. how format toggles work (sdjwtietf / sdjwtw3c / jwtw3c)
//  3. what the per-format issuance form renders
//
// Then dump the same observations for OUR /issuer/schema page so we can
// compare shape-for-shape instead of guessing.

import puppeteer from 'puppeteer-core';

const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';
const LOCAL = 'http://localhost:8080';
const PORTAL = 'https://portal.walt.id';

const browser = await puppeteer.launch({
  executablePath: CHROME, headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});

async function dump(page, label) {
  const info = await page.evaluate(() => {
    const text = document.body.innerText.slice(0, 1500);
    const buttons = [...document.querySelectorAll('button, [role="tab"], [role="button"]')]
      .map((b) => (b.textContent || '').trim()).filter(Boolean).slice(0, 40);
    const inputs = [...document.querySelectorAll('input, select, textarea')]
      .map((i) => ({ tag: i.tagName, type: i.type, name: i.name, placeholder: i.placeholder }))
      .slice(0, 30);
    return { url: location.href, text, buttons, inputs };
  });
  console.log(`\n========== ${label} ==========`);
  console.log('URL:', info.url);
  console.log('Text head:');
  console.log(info.text.split('\n').slice(0, 20).join('\n'));
  console.log('Buttons:', info.buttons.slice(0, 20).join(' | '));
  console.log('Inputs:', JSON.stringify(info.inputs.slice(0, 10)));
}

// Walt.id's portal — try to reach the issuer wizard.
async function probePortal() {
  const page = await browser.newPage();
  const net = [];
  page.on('response', async (r) => {
    const u = r.url();
    if (/credentials\.walt\.id\/api\/|portal\.walt\.id\/api/.test(u))
      net.push({ s: r.status(), u });
  });
  await page.goto(PORTAL, { waitUntil: 'networkidle2', timeout: 45000 });
  await new Promise((r) => setTimeout(r, 2000));
  await dump(page, 'portal landing');

  // Try to reach an Issue page.
  await page.evaluate(() => {
    const el = [...document.querySelectorAll('a, button')].find((e) =>
      /^\s*issue\s*$/i.test(e.textContent || ''));
    el?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 900, timeout: 10000 }).catch(() => {});
  await dump(page, 'portal after Issue click');

  // Click any credential tile we can find (OpenBadge is their canonical one).
  await page.evaluate(() => {
    const el = [...document.querySelectorAll('[class*="card" i], [class*="tile" i], a, button')]
      .find((e) => /openbadge/i.test(e.textContent || ''));
    el?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 900, timeout: 10000 }).catch(() => {});
  await dump(page, 'portal OpenBadge detail');

  // Walk the page looking for format-toggle chips.
  const formats = await page.evaluate(() => {
    const matches = [];
    for (const e of document.querySelectorAll('*')) {
      const t = (e.textContent || '').trim().toLowerCase();
      if (/^(jwt\s*vc|jwt_vc_json|sd[_-]?jwt|ldp[_-]?vc|vc\+sd-jwt|mso_mdoc|w3c\s*vc|jwtw3c|sdjwtietf|sdjwtw3c)$/.test(t)) {
        matches.push({ tag: e.tagName, role: e.getAttribute('role'), text: t });
      }
    }
    return matches.slice(0, 30);
  });
  console.log('\nformat-toggle-shaped elements:', JSON.stringify(formats, null, 2));

  // Fetch the IdentityCredential template to see its real shape
  const template = await page.evaluate(async () => {
    const r = await fetch('https://credentials.walt.id/api/vc/IdentityCredential');
    return await r.json();
  });
  console.log('\ncredentials.walt.id IdentityCredential template:');
  console.log(JSON.stringify(template, null, 2).slice(0, 2000));

  console.log('\n-- portal API calls observed --');
  for (const c of net.slice(0, 40)) console.log(` ${c.s}  ${c.u}`);
  await page.close();
}

async function probeLocal() {
  const page = await browser.newPage();
  // Must auth first — jump to /issuer/dpg to see what we render without a session.
  await page.goto(LOCAL + '/issuer/schema', { waitUntil: 'networkidle2' });
  await new Promise((r) => setTimeout(r, 1200));
  await dump(page, 'local /issuer/schema (no session)');
  await page.close();
}

try {
  await probePortal();
  await probeLocal();
} catch (e) {
  console.error('FATAL:', e.message);
} finally {
  await browser.close();
}
