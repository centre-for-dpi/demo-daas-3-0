// Verify selective disclosure: build a verifier request for OpenBadgeCredential
// (vc+sd-jwt) with only [holder, issuedOn] checked (achievement unchecked).
// Fetch the PD and confirm it carries limit_disclosure=required + one path
// per checked field + the vct filter.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const page = await br.newPage();
await page.setViewport({ width: 1500, height: 1200 });
async function auth() {
  await page.goto('http://localhost:8080/', { waitUntil: 'networkidle2' });
  await page.click('button.role-card[value="verifier"]');
  await page.waitForFunction(() => /\/auth/.test(location.pathname), { timeout: 10000 });
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(() => null),
    page.evaluate(() => [...document.querySelectorAll('button.provider-btn')].find(b => (b.getAttribute('hx-vals')||'').includes('keycloak'))?.click()),
  ]);
  await page.waitForSelector('input[name="username"]', { timeout: 15000 });
  await page.type('input[name="username"]', 'admin');
  await page.type('input[name="password"]', 'admin');
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(() => null),
    page.click('input[type="submit"], button[type="submit"]'),
  ]);
  await new Promise(r => setTimeout(r, 700));
}
try {
  await auth();
  if (!/verifier\/verify/.test(page.url())) {
    await page.goto('http://localhost:8080/verifier/dpg', { waitUntil: 'networkidle2' });
    if (await page.$('.dpg-card[data-vendor="Walt Community Stack"]')) {
      await page.click('.dpg-card[data-vendor="Walt Community Stack"]');
      await new Promise(r => setTimeout(r, 300));
      await page.click('#verifier-dpg-continue').catch(()=>{});
      await new Promise(r => setTimeout(r, 900));
    }
  }
  await page.goto('http://localhost:8080/verifier/verify', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 800));

  // Pick Open Badge Credential → vc+sd-jwt
  await page.evaluate(() => {
    const card = [...document.querySelectorAll('#verifier-custom-body .schema-card')]
      .find(c => c.dataset.name === 'Open Badge Credential');
    const chip = [...card.querySelectorAll('.chip.small')].find(x => x.title === 'vc+sd-jwt');
    chip?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 500));

  // Uncheck "achievement"
  await page.evaluate(() => {
    const cb = [...document.querySelectorAll('input[name="field_key"]')]
      .find(i => i.value === 'achievement');
    if (cb && cb.checked) cb.click();
  });
  await new Promise(r => setTimeout(r, 200));

  // Generate
  await page.evaluate(() => {
    const btn = [...document.querySelectorAll('#custom-template-form button[type="submit"]')]
      .find(b => /Generate/i.test(b.textContent));
    btn?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 1000));

  const authURI = await page.evaluate(() => document.querySelector('#oid4vp-output .link-display')?.textContent?.trim());
  console.log('auth URI:', authURI?.slice(0, 120) + '…');

  const pdURI = new URL(authURI).searchParams.get('presentation_definition_uri');
  const pd = await (await fetch(pdURI)).json();
  console.log('\nPD:');
  console.log(JSON.stringify(pd, null, 2));

  const descriptor = pd.input_descriptors?.[0];
  const paths = descriptor?.constraints?.fields?.map(f => f.path?.[0]);
  const limit = descriptor?.constraints?.limit_disclosure;
  console.log('\npaths:', paths);
  console.log('limit_disclosure:', limit);
  console.log('expected: [$.vct (with filter), $.holder, $.issuedOn], limit=required');
} finally {
  await br.close();
}
