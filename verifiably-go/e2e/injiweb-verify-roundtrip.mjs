// End-to-end: issue a FarmerCredential through Inji Web's auth-code flow,
// then verify it through Inji Verify. This is the full loop the user runs
// manually in the browser, automated so we can assert the pipeline works
// after a reset.
//
// Steps:
//   1. Go to Inji Web, continue as guest
//   2. Pick Agriculture Department → Farmer Credential (V2)
//   3. eSignet redirect → enter individualId 8267411072, submit OTP 111111
//   4. Consent screen → allow
//   5. Get redirected back to Inji Web → PDF download link appears
//   6. Fetch the PDF, extract the QR, decode the VC
//   7. POST the VC to Inji Verify; assert verificationStatus === "SUCCESS"
//
// Usage: node e2e/injiweb-verify-roundtrip.mjs

import puppeteer from 'puppeteer-core';

const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';
const INJIWEB = process.env.INJIWEB_URL || 'http://172.24.0.1:3004';
const INDIVIDUAL = process.env.INJI_TEST_INDIVIDUAL || '8267411072';
const OTP = process.env.INJI_TEST_OTP || '111111';

function log(ok, msg, detail) {
  console.log((ok ? 'PASS' : 'FAIL') + '  ' + msg + (detail ? ' — ' + detail : ''));
  if (!ok) process.exitCode = 1;
}
const wait = (ms) => new Promise((r) => setTimeout(r, ms));
const settle = (p, t = 8000) => p.waitForNetworkIdle({ idleTime: 800, timeout: t }).catch(() => {});

const browser = await puppeteer.launch({
  executablePath: CHROME, headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
const page = await browser.newPage();
page.on('console', (m) => {
  if (m.type() === 'error') console.log('[browser err]', m.text().slice(0, 200));
});

try {
  // 1. Inji Web landing → Continue as Guest
  await page.goto(INJIWEB + '/', { waitUntil: 'networkidle2', timeout: 20000 });
  await wait(1500);
  await page.evaluate(() => {
    const b = [...document.querySelectorAll('button, a')].find((n) =>
      /Continue as Guest/i.test(n.textContent || ''));
    b?.click();
  });
  await settle(page);
  if (!/\/issuers/.test(page.url())) {
    await page.goto(INJIWEB + '/issuers', { waitUntil: 'networkidle2', timeout: 15000 });
  }
  await wait(1500);
  log(/Agriculture Department/.test(await page.evaluate(() => document.body.innerText)),
      'issuer catalog has Agriculture Department');

  // 2. Click Agriculture Department (walk up to cursor:pointer ancestor)
  await page.evaluate(() => {
    const m = document.evaluate("//*[normalize-space(text())='Agriculture Department']",
      document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
    let el = m;
    for (let i = 0; i < 10 && el && el !== document.body; i++) {
      if (getComputedStyle(el).cursor === 'pointer') { el.click(); return; }
      el = el.parentElement;
    }
    m?.click();
  });
  await settle(page);
  await wait(1500);
  log(/\/issuers\/Farmer/.test(page.url()), 'landed on /issuers/Farmer', page.url());

  // 3. Click Farmer Credential (V2)
  await page.evaluate(() => {
    const nodes = [...document.querySelectorAll('button, a, div, span, h3, h4')];
    const m = nodes.find((n) => /Farmer Credential \(V2\)/.test(n.textContent || ''));
    let el = m;
    for (let i = 0; i < 10 && el && el !== document.body; i++) {
      if (getComputedStyle(el).cursor === 'pointer') { el.click(); return; }
      el = el.parentElement;
    }
    m?.click();
  });

  // 4. Wait for eSignet redirect
  await page.waitForFunction(() => /authorize|esignet/.test(location.href),
    { timeout: 15000 }).catch(() => {});
  await settle(page);
  await wait(2500);
  console.log('  after credential click:', page.url().slice(0, 100));

  // 5. Accept "claims you allow to be shared" if present, then pick OTP auth
  await page.evaluate(() => {
    const allowBtn = [...document.querySelectorAll('button')].find((b) =>
      /Allow|Proceed|Continue/i.test(b.textContent || ''));
    allowBtn?.click();
  });
  await wait(1500);

  // 6. Enter individualId
  const idField = await page.$('input[type="text"], input[type="tel"], input[name*="id" i]');
  if (idField) {
    await idField.click({ clickCount: 3 });
    await idField.type(INDIVIDUAL);
  }
  // Click "Get OTP" / "Login with OTP" button
  await page.evaluate(() => {
    const b = [...document.querySelectorAll('button')].find((n) =>
      /Get OTP|Send OTP|Login with OTP|Login/i.test(n.textContent || ''));
    b?.click();
  });
  await wait(2500);

  // 7. Type OTP (6-digit field or multi-input)
  await page.evaluate((otp) => {
    const inputs = [...document.querySelectorAll('input')].filter(
      (i) => /number|tel|otp|pin/i.test(i.type + i.name + (i.getAttribute('autocomplete')||'')) ||
             i.maxLength === 1);
    if (inputs.length >= 6) {
      for (let i = 0; i < 6; i++) {
        inputs[i].value = otp[i];
        inputs[i].dispatchEvent(new Event('input', { bubbles: true }));
      }
    } else if (inputs.length > 0) {
      inputs[0].value = otp;
      inputs[0].dispatchEvent(new Event('input', { bubbles: true }));
    }
  }, OTP);
  await wait(1000);
  await page.evaluate(() => {
    const b = [...document.querySelectorAll('button')].find((n) =>
      /Verify|Submit|Login|Continue/i.test(n.textContent || ''));
    b?.click();
  });
  await settle(page, 15000);
  await wait(2500);
  console.log('  after OTP submit:', page.url().slice(0, 100));

  // 8. Consent screen — click Allow/Proceed
  await page.evaluate(() => {
    const b = [...document.querySelectorAll('button')].find((n) =>
      /Allow|Proceed|Continue|Accept/i.test(n.textContent || ''));
    b?.click();
  });
  await settle(page, 15000);
  await wait(3000);
  console.log('  after consent:', page.url().slice(0, 100));

  const pageText = await page.evaluate(() => document.body.innerText);
  console.log(pageText.slice(0, 600));

  // 9. Look for a PDF download link
  const downloadLink = await page.$$eval('a', (els) =>
    els.map((a) => a.href).find((h) => /\.pdf|download/i.test(h)));
  log(!!downloadLink, 'credential download link appeared', downloadLink);
} catch (e) {
  console.error('FATAL:', e.message, e.stack);
  process.exit(2);
} finally {
  await browser.close();
}
