// Headless regression for FX5 — "No Credentials found" when opening Inji Web
// from verifiably-go. Root cause: the SPA's MIMOTO_URL is injected as
// http://${PUBLIC_HOST}:3004/v1/mimoto (172.24.0.1 in the shared .env), so if
// the user opens the SPA on a DIFFERENT origin (e.g. localhost), every XHR is
// cross-origin and the browser blocks the responses — UI falls back to "No
// Credentials found". Fix: verifiably-go links to http://172.24.0.1:3004 so
// the SPA's XHRs stay same-origin.
//
// This test walks the full path: guest login → issuer list → pick Agriculture
// Department → assert the three Farmer credential types render.

import puppeteer from 'puppeteer-core';

const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';
const INJIWEB = process.env.INJIWEB_URL || 'http://172.24.0.1:3004';

function log(ok, msg, detail) {
  console.log((ok ? 'PASS' : 'FAIL') + '  ' + msg + (detail ? ' — ' + detail : ''));
  if (!ok) process.exitCode = 1;
}

const browser = await puppeteer.launch({
  executablePath: CHROME, headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
const page = await browser.newPage();

try {
  await page.goto(INJIWEB + '/', { waitUntil: 'networkidle2', timeout: 20000 });
  await new Promise((r) => setTimeout(r, 1200));

  await page.evaluate(() => {
    const btn = [...document.querySelectorAll('button, a')].find((n) =>
      /Continue as Guest/i.test(n.textContent || ''));
    btn?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 700, timeout: 8000 }).catch(() => {});
  if (!/\/issuers/.test(page.url())) {
    await page.goto(INJIWEB + '/issuers', { waitUntil: 'networkidle2', timeout: 15000 });
  }
  await new Promise((r) => setTimeout(r, 1200));

  const hasAgri = await page.evaluate(() =>
    /Agriculture Department/i.test(document.body.innerText));
  log(hasAgri, 'issuer catalog rendered with Agriculture Department');
  if (!hasAgri) process.exit(1);

  await page.evaluate(() => {
    const match = document.evaluate(
      "//*[normalize-space(text())='Agriculture Department']",
      document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
    let el = match;
    for (let i = 0; i < 10 && el && el !== document.body; i++) {
      if (getComputedStyle(el).cursor === 'pointer') { el.click(); return; }
      el = el.parentElement;
    }
    match?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 700, timeout: 10000 }).catch(() => {});
  await new Promise((r) => setTimeout(r, 1500));

  const text = await page.evaluate(() => document.body.innerText);
  const urlOk = /\/issuers\/Farmer/.test(page.url());
  log(urlOk, 'navigated to /issuers/Farmer', page.url());

  const hasNone = /No Credentials? (found|available)/i.test(text);
  log(!hasNone, 'no "No Credentials found" message');

  for (const needle of [
    'Farmer Credential (V2)',
    'Farmer Credential (SD-JWT)',
    'Farmer Verifiable Credential',
  ]) {
    log(text.includes(needle), `credential "${needle}" rendered`);
  }
} catch (e) {
  console.error('FATAL:', e.message);
  process.exit(2);
} finally {
  await browser.close();
}
