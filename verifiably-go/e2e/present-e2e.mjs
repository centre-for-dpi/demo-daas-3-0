// Drive the present flow through the real UI (Keycloak login + walt.id
// wallet) and capture verifiably-go's server logs for the PresentCredential
// call. Tells us exactly what id + URL our adapter submits to walt.id.
//
// Requires deploy.sh run all to have provisioned Keycloak with the demo
// user (any user in the vcplatform realm — admin/admin works if seeded).

import puppeteer from 'puppeteer-core';

const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';
const BASE = process.env.VERIFIABLY_URL || 'http://localhost:8080';
const KC_USER = process.env.KC_USER || 'admin';
const KC_PASS = process.env.KC_PASS || 'admin';

const browser = await puppeteer.launch({
  executablePath: CHROME, headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage', '--ignore-certificate-errors'],
});
const page = await browser.newPage();
page.on('pageerror', (e) => console.log('[browser err]', e.message));

async function settle(t = 6000) { await page.waitForNetworkIdle({ idleTime: 600, timeout: t }).catch(() => {}); }

try {
  // 1. Land, pick holder role.
  await page.goto(BASE + '/', { waitUntil: 'networkidle2' });
  await page.click('button.role-card[value="holder"]');
  await settle();
  await page.waitForFunction(() => /\/auth/.test(location.pathname), { timeout: 10000 });

  // 2. Click Keycloak provider.
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(() => null),
    page.evaluate(() => {
      const btn = [...document.querySelectorAll('button.provider-btn')].find(
        (b) => (b.getAttribute('hx-vals') || '').includes('"keycloak"'));
      btn?.click();
    }),
  ]);
  console.log('[nav]', page.url());
  await settle();

  // 3. Keycloak login.
  await page.waitForSelector('input[name="username"]', { timeout: 15000 });
  await page.type('input[name="username"]', KC_USER);
  await page.type('input[name="password"]', KC_PASS);
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(() => null),
    page.click('input[type="submit"], button[type="submit"]'),
  ]);
  console.log('[post-login]', page.url());
  await settle();

  // 4. DPG pick: walt.id.
  if (!/holder\/dpg|holder\/wallet/.test(page.url())) {
    await page.goto(BASE + '/holder/dpg', { waitUntil: 'networkidle2' });
  }
  await settle();
  if (await page.$('.dpg-card[data-vendor="Walt Community Stack"]')) {
    await page.click('.dpg-card[data-vendor="Walt Community Stack"]');
    await settle();
    await page.click('#holder-dpg-continue').catch(() => {});
    await settle();
  }
  console.log('[wallet]', page.url());

  // 5. Generate a fresh verifier request via our own verifier-api curl loop.
  const authURI = await fetch('http://localhost:7003/openid4vc/verify', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      request_credentials: [{ format: 'jwt_vc_json', type: 'IdentityCredential' }],
      vp_policies: ['signature', 'presentation-definition'],
    }),
  }).then((r) => r.text());
  console.log('[verifier] auth URI:', authURI.slice(0, 120));

  // 6. Go to /holder/present, paste it, submit.
  await page.goto(BASE + '/holder/present', { waitUntil: 'networkidle2' });
  await settle();
  await page.evaluate((uri) => {
    const ta = document.querySelector('textarea[name="request_uri"]');
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
    setter.call(ta, uri);
    ta.dispatchEvent(new Event('input', { bubbles: true }));
  }, authURI);
  console.log('[form] pasted URI');

  // Which credential is picked in the dropdown?
  const picked = await page.evaluate(() => {
    const sel = document.querySelector('select[name="credential_id"]');
    return sel ? { value: sel.value, text: sel.options[sel.selectedIndex]?.text } : null;
  });
  console.log('[form] credential:', picked);

  // Click submit.
  await page.evaluate(() => {
    const b = [...document.querySelectorAll('button[type="submit"]')].find((x) => /Send/i.test(x.textContent || ''));
    b?.click();
  });
  await settle(15000);
  await new Promise((r) => setTimeout(r, 2000));

  const result = await page.evaluate(() => document.getElementById('present-result')?.innerText || '');
  console.log('[result]:', result.slice(0, 400));
} catch (e) {
  console.error('FATAL:', e.message);
} finally {
  await browser.close();
}
