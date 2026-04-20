// Full verifier flow with policy checkboxes + flip-card result:
// 1. Sign in as verifier, pick walt.id DPG
// 2. Pick Identity Credential (vc+sd-jwt), check fields, enable policies
// 3. Generate request
// 4. Issue a real credential + present it via wallet-api (drives the holder side)
// 5. Check for response, verify the flip-card result + disclosed fields
import puppeteer from 'puppeteer-core';

const BASE = 'http://localhost:8080';
const KC_USER = 'admin', KC_PASS = 'admin';

const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox','--disable-dev-shm-usage'] });
const page = await br.newPage();
await page.setViewport({ width: 1600, height: 1400 });

async function keycloakAuth(page, role) {
  await page.goto(BASE + '/', { waitUntil: 'networkidle2' });
  await page.click(`button.role-card[value="${role}"]`);
  await page.waitForFunction(() => /\/auth/.test(location.pathname), { timeout: 10000 });
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(() => null),
    page.evaluate(() => [...document.querySelectorAll('button.provider-btn')].find(b => (b.getAttribute('hx-vals')||'').includes('keycloak'))?.click()),
  ]);
  await page.waitForSelector('input[name="username"]', { timeout: 15000 });
  await page.type('input[name="username"]', KC_USER);
  await page.type('input[name="password"]', KC_PASS);
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(() => null),
    page.click('input[type="submit"], button[type="submit"]'),
  ]);
  await new Promise(r => setTimeout(r, 700));
}

try {
  await keycloakAuth(page, 'verifier');
  if (!/verifier\/verify/.test(page.url())) {
    await page.goto(BASE + '/verifier/dpg', { waitUntil: 'networkidle2' });
    if (await page.$('.dpg-card[data-vendor="Walt Community Stack"]')) {
      await page.click('.dpg-card[data-vendor="Walt Community Stack"]');
      await new Promise(r => setTimeout(r, 300));
      await page.click('#verifier-dpg-continue').catch(()=>{});
      await new Promise(r => setTimeout(r, 800));
    }
  }
  await page.goto(BASE + '/verifier/verify', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 800));

  // Pick Identity Credential + vc+sd-jwt format chip
  await page.evaluate(() => {
    const card = [...document.querySelectorAll('#verifier-custom-body .schema-card')]
      .find(c => c.dataset.name === 'Identity Credential');
    const chip = [...card.querySelectorAll('.chip.small')].find(x => x.title === 'vc+sd-jwt');
    chip?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 500));

  // Verify policies are default-checked
  const policyState = await page.evaluate(() => {
    const boxes = [...document.querySelectorAll('input[name="policy"]')];
    return boxes.map(b => ({ value: b.value, checked: b.checked }));
  });
  console.log('policy boxes:', JSON.stringify(policyState));

  // Generate the request
  await page.evaluate(() => {
    const btn = [...document.querySelectorAll('#custom-template-form button[type="submit"]')]
      .find(b => /Generate/i.test(b.textContent));
    btn?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 1200));

  const authURI = await page.evaluate(() => document.querySelector('#oid4vp-output .link-display')?.textContent?.trim());
  console.log('verifier URI:', authURI?.slice(0, 120) + '…');

  // Drive the holder side in a fresh INCOGNITO context so app sessions /
  // cookies don't collide with the verifier's.
  const holderCtx = await br.createBrowserContext();
  const holderPage = await holderCtx.newPage();
  await holderPage.setViewport({ width: 1400, height: 1100 });
  await keycloakAuth(holderPage, 'holder');
  // Pick walt.id as holder DPG, then navigate to /holder/present
  if (!/holder\/wallet|holder\/present/.test(holderPage.url())) {
    await holderPage.goto(BASE + '/holder/dpg', { waitUntil: 'networkidle2' });
    if (await holderPage.$('.dpg-card[data-vendor="Walt Community Stack"]')) {
      await holderPage.click('.dpg-card[data-vendor="Walt Community Stack"]');
      await new Promise(r => setTimeout(r, 300));
      await holderPage.click('#holder-dpg-continue').catch(()=>{});
      await new Promise(r => setTimeout(r, 800));
    }
  }

  // Issue a fresh IdentityCredential to the wallet (via a direct verifier-API loop)
  // — use our own /issuer/issue API? Simpler: just call walt.id issuer + claim via wallet.
  // But we already have BootstrapOffers that creates a seed cred. Let's check holder wallet for any Identity Credential.
  await holderPage.goto(BASE + '/holder/wallet', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 700));
  const holderCreds = await holderPage.evaluate(() => {
    return [...document.querySelectorAll('.cred-card, [data-cred-id]')].map(c => ({
      id: c.dataset.credId || c.dataset.id,
      text: (c.textContent || '').slice(0, 150).replace(/\s+/g, ' ').trim(),
    }));
  });
  console.log('\nholder creds (first 3):', JSON.stringify(holderCreds.slice(0, 3), null, 2));
  // If no IdentityCredential — accept one via the issuer flow. Skip for now.

  // Present: go to /holder/present, paste the verifier URI, submit.
  await holderPage.goto(BASE + '/holder/present', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 500));
  await holderPage.evaluate((uri) => {
    const ta = document.querySelector('textarea[name="request_uri"]');
    if (!ta) return;
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
    setter.call(ta, uri);
    ta.dispatchEvent(new Event('input', { bubbles: true }));
  }, authURI);
  const pickedCred = await holderPage.evaluate(() => {
    const sel = document.querySelector('select[name="credential_id"]');
    return sel ? { value: sel.value, count: sel.options.length } : null;
  });
  console.log('holder picked cred:', JSON.stringify(pickedCred));
  await holderPage.evaluate(() => {
    const b = [...document.querySelectorAll('button[type="submit"]')].find(x => /Send/i.test(x.textContent || ''));
    b?.click();
  });
  await holderPage.waitForNetworkIdle({ idleTime: 600, timeout: 15000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 2000));
  const presentOutcome = await holderPage.evaluate(() => (document.getElementById('present-result')?.innerText || '').slice(0, 200));
  console.log('holder present outcome:', presentOutcome);
  await holderPage.close();
  await holderCtx.close();

  // Now back on the verifier page, click Check for response
  await page.bringToFront();
  await page.evaluate(() => {
    const b = [...document.querySelectorAll('button')].find(x => /Check for holder response/i.test(x.textContent || ''));
    b?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 600, timeout: 15000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 2500));

  const verifyResult = await page.evaluate(() => {
    const flip = document.querySelector('.verify-flip');
    const front = flip?.querySelector('.verify-flip-front');
    const back = flip?.querySelector('.verify-flip-back');
    const pending = document.querySelector('.verify-result.pending');
    const resultDiv = document.getElementById('verify-result');
    return {
      has_flip_card: !!flip,
      pending: !!pending,
      pending_text: (pending?.innerText || '').slice(0, 400),
      result_div_text: (resultDiv?.innerText || '').slice(0, 500),
      valid_class: flip?.classList.contains('valid'),
      invalid_class: flip?.classList.contains('invalid'),
      front_heading: front?.querySelector('h3')?.textContent?.trim(),
      front_policies: [...front?.querySelectorAll('.pill') || []].map(p => p.textContent.trim()),
      front_text_head: (front?.innerText || '').slice(0, 500),
      back_heading: back?.querySelector('h3')?.textContent?.trim(),
      back_text: (back?.innerText || '').slice(0, 600),
    };
  });
  console.log('\n=== verification result ===');
  console.log(JSON.stringify(verifyResult, null, 2));
} catch (e) {
  console.error('FATAL:', e.message);
} finally {
  await br.close();
}
