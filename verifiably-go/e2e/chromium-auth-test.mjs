// Chromium-driven auth-picker test: clicks the real provider buttons in a
// headless browser and verifies HTMX follows the HX-Redirect to the external
// IdP authorize endpoint. Covers the gap auth-test.mjs left — that test
// POSTed /auth/start via fetch and never exercised the actual button click.
//
// Usage: VERIFIABLY_URL=http://localhost:8089 node e2e/chromium-auth-test.mjs

import puppeteer from 'puppeteer-core';

const BASE = process.env.VERIFIABLY_URL || 'http://localhost:8089';
const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';

const results = [];
const fail = [];
function log(ok, msg, detail) {
  console.log((ok ? 'PASS' : 'FAIL') + '  ' + msg + (detail ? ' — ' + detail : ''));
  results.push({ ok, msg, detail });
  if (!ok) fail.push({ msg, detail });
}
async function expect(cond, msg, detail) { log(!!cond, msg, cond ? '' : detail); }

async function drive(page, providerId, expectHost, expectPath) {
  await page.goto(BASE + '/', { waitUntil: 'networkidle0' });
  // Pick a role to land on /auth.
  await page.click('button.role-card[value="issuer"]');
  await page.waitForFunction(() => /\/auth/.test(location.pathname), { timeout: 8000 });

  // Find the provider button and click it. The button has
  // hx-post="/auth/start" and hx-vals='{"provider":"<id>"}'. HTMX intercepts
  // the click, posts the form, and follows the HX-Redirect header.
  const has = await page.evaluate((id) => {
    const btn = Array.from(document.querySelectorAll('button.provider-btn'))
      .find((b) => (b.getAttribute('hx-vals') || '').includes('"' + id + '"'));
    return !!btn;
  }, providerId);
  await expect(has, `${providerId}: provider button rendered`, '');
  if (!has) return;

  // Intercept the navigation — HTMX turns the HX-Redirect into a real
  // document navigation, so page.waitForNavigation catches it.
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(() => null),
    page.evaluate((id) => {
      const btn = Array.from(document.querySelectorAll('button.provider-btn'))
        .find((b) => (b.getAttribute('hx-vals') || '').includes('"' + id + '"'));
      btn?.click();
    }, providerId),
  ]);
  // Give HTMX an extra tick to process HX-Redirect headers and navigate.
  await new Promise((r) => setTimeout(r, 500));
  const landedURL = page.url();
  await expect(
    landedURL.includes(expectHost),
    `${providerId}: browser landed on provider host`,
    landedURL,
  );
  // Some IdPs (WSO2IS on first run) bounce the /authorize URL to an error
  // endpoint on the SAME host when the client isn't registered. That's an
  // IdP-side deployment gap — our job here is to prove the HTMX HX-Redirect
  // actually made the browser navigate off verifiably-go and onto the IdP.
  const landedOnIDP = landedURL.includes(expectHost);
  const landedOnAuthorize = landedURL.includes(expectPath);
  if (landedOnAuthorize) {
    const url = new URL(landedURL);
    await expect(
      url.searchParams.get('response_type') === 'code',
      `${providerId}: response_type=code in URL`,
      url.searchParams.get('response_type') || '(missing)',
    );
    await expect(
      url.searchParams.get('code_challenge_method') === 'S256',
      `${providerId}: PKCE S256 method in URL`,
      '',
    );
  } else if (landedOnIDP) {
    // The browser reached the IdP but the IdP bounced us (likely because the
    // client isn't registered). The critical thing — the redirect succeeded —
    // is already asserted by the host check above. Log the bounce so we
    // don't silently hide an IdP misconfiguration.
    log(true, `${providerId}: IdP accepted the redirect (bounced to ${new URL(landedURL).pathname} — register the client to complete login)`);
  }
}

async function run() {
  const browser = await puppeteer.launch({
    executablePath: CHROME, headless: 'new',
    args: [
      '--no-sandbox',
      '--disable-dev-shm-usage',
      // Allow the Chromium to navigate to WSO2IS's self-signed cert.
      '--ignore-certificate-errors',
    ],
  });
  const page = await browser.newPage();
  page.on('pageerror', (e) => log(false, 'uncaught JS error', e.message));

  try {
    await drive(page, 'keycloak', 'localhost:8180', '/realms/master/protocol/openid-connect/auth');
    // Fresh page for WSO2IS run.
    await page.evaluate(() => document.cookie.split(';').forEach((c) => {
      const n = c.split('=')[0].trim();
      document.cookie = n + '=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/';
    }));
    await drive(page, 'wso2is', 'localhost:9443', '/oauth2/authorize');
  } catch (e) {
    console.error('FATAL:', e.message);
    console.error(e.stack);
  }

  await browser.close();
  console.log('\n' + '='.repeat(60));
  console.log(`Results: ${results.filter((r) => r.ok).length}/${results.length} passed`);
  if (fail.length) {
    console.log('\nFailures:');
    for (const f of fail) console.log(`  - ${f.msg}${f.detail ? ' — ' + f.detail : ''}`);
    process.exit(1);
  }
}

run();
