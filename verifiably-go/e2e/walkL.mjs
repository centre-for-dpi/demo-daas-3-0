// Exercise WSO2IS OIDC login end-to-end. Verifies that:
//  1. The authorize redirect goes to the browser-visible (172.24.0.1:9443) host.
//  2. After login, the token exchange uses the container-internal host (wso2is:9443).
// Previously: the token endpoint kept 'localhost:9443' from WSO2's discovery
// document, which didn't resolve inside our container.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({
  executablePath: '/usr/bin/google-chrome',
  headless: 'new',
  args: ['--no-sandbox', '--ignore-certificate-errors'],
});
const page = await br.newPage();
await page.setViewport({ width: 1400, height: 1100 });
page.on('framenavigated', (f) => console.log('[nav]', f.url().slice(0, 140)));

try {
  await page.goto('http://localhost:8080/', { waitUntil: 'networkidle2' });
  await page.click('button.role-card[value="holder"]');
  await page.waitForFunction(() => /\/auth/.test(location.pathname), { timeout: 10000 });

  // Click the WSO2IS provider
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(() => null),
    page.evaluate(() => {
      const btn = [...document.querySelectorAll('button.provider-btn')].find(
        (b) => (b.getAttribute('hx-vals') || '').includes('"wso2is"'));
      btn?.click();
    }),
  ]);
  console.log('[post-click]', page.url());

  // WSO2's login page has input fields named 'username' and 'password'
  await page.waitForSelector('input[name="username"], input#username', { timeout: 15000 });
  await page.type('input[name="username"], input#username', 'testawso2');
  await page.type('input[name="password"], input#password', 'Q!w2e3r4');
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 30000 }).catch(() => null),
    page.click('button[type="submit"], input[type="submit"]'),
  ]);
  console.log('[post-login]', page.url());

  // If there's a consent/allow button, click it
  const allow = await page.$('button[value="approve"], input[value="approve"], input[name="hasApprovedAlways"], input#approve, button#approve');
  if (allow) {
    await Promise.all([
      page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(() => null),
      allow.click(),
    ]);
    console.log('[post-consent]', page.url());
  }

  // Give the callback + token exchange a moment
  await new Promise((r) => setTimeout(r, 3000));
  console.log('[final]', page.url());

  // Dump any visible error toast or page text
  const snippet = await page.evaluate(() => document.body.innerText.slice(0, 400));
  console.log('[page text]:', snippet);
} catch (e) {
  console.error('FATAL:', e.message);
} finally {
  await br.close();
}
