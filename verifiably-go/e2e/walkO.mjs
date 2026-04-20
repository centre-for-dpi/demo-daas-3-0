// Exercise the new consent interstitial:
//  1. Build a verifier request for OpenBadgeCredential / vc+sd-jwt with
//     only `holder` + `issuedOn` checked.
//  2. In an incognito holder session, paste the URI and pick a credential.
//  3. Verify the consent fragment renders with the right fields, verifier,
//     and disclosure badge — WITHOUT the adapter submitting to walt.id yet.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox', '--disable-dev-shm-usage'] });

async function auth(page, role) {
  await page.goto('http://localhost:8080/', { waitUntil: 'networkidle2' });
  await page.click(`button.role-card[value="${role}"]`);
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

async function pickDPG(page, role) {
  await page.goto(`http://localhost:8080/${role}/dpg`, { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 400));
  const card = await page.$('.dpg-card[data-vendor="Walt Community Stack"]');
  if (card) {
    await card.click();
    await new Promise(r => setTimeout(r, 400));
    await page.click(`#${role}-dpg-continue`).catch(() => {});
    await new Promise(r => setTimeout(r, 900));
  }
}

try {
  // ==== Verifier side ====
  const vCtx = await br.createBrowserContext();
  const vPage = await vCtx.newPage();
  await vPage.setViewport({ width: 1500, height: 1100 });
  await auth(vPage, 'verifier');
  await pickDPG(vPage, 'verifier');
  await vPage.goto('http://localhost:8080/verifier/verify', { waitUntil: 'networkidle2' });
  await vPage.waitForSelector('#verifier-custom-body .schema-card', { timeout: 15000 });
  await new Promise(r => setTimeout(r, 400));
  await vPage.evaluate(() => {
    const card = [...document.querySelectorAll('#verifier-custom-body .schema-card')]
      .find(c => c.dataset.name === 'Open Badge Credential');
    if (!card) throw new Error('Open Badge Credential card not present');
    const chip = [...card.querySelectorAll('.chip.small')].find(x => x.title === 'vc+sd-jwt');
    chip?.click();
  });
  await vPage.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 400));
  // Uncheck `achievement`
  await vPage.evaluate(() => {
    const cb = [...document.querySelectorAll('input[name="field_key"]')].find(i => i.value === 'achievement');
    if (cb && cb.checked) cb.click();
  });
  await new Promise(r => setTimeout(r, 200));
  await vPage.evaluate(() => {
    const b = [...document.querySelectorAll('#custom-template-form button[type="submit"]')].find(x => /Generate/i.test(x.textContent));
    b?.click();
  });
  await vPage.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 800));
  const authURI = await vPage.evaluate(() => document.querySelector('#oid4vp-output .link-display')?.textContent?.trim());
  console.log('[verifier] auth URI head:', authURI?.slice(0, 90) + '…');
  await vPage.close();
  await vCtx.close();

  // ==== Holder side ====
  const hCtx = await br.createBrowserContext();
  const hPage = await hCtx.newPage();
  await hPage.setViewport({ width: 1400, height: 1100 });
  await auth(hPage, 'holder');
  await pickDPG(hPage, 'holder');
  await hPage.goto('http://localhost:8080/holder/present', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 500));

  // Paste URI + pick the first credential
  await hPage.evaluate((uri) => {
    const ta = document.querySelector('textarea[name="request_uri"]');
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
    setter.call(ta, uri);
    ta.dispatchEvent(new Event('input', { bubbles: true }));
  }, authURI);
  const picked = await hPage.evaluate(() => {
    const sel = document.querySelector('select[name="credential_id"]');
    if (!sel) return null;
    // Prefer an Open Badge if present, else the first
    for (const o of sel.options) {
      if (/open badge/i.test(o.textContent)) { sel.value = o.value; break; }
    }
    return { value: sel.value, text: sel.options[sel.selectedIndex]?.text };
  });
  console.log('[holder] picked cred:', picked);

  // Click Review & send — should render the CONSENT fragment, not submit.
  await hPage.evaluate(() => {
    const b = [...document.querySelectorAll('button[type="submit"]')].find(x => /Review/i.test(x.textContent || ''));
    b?.click();
  });
  await hPage.waitForNetworkIdle({ idleTime: 600, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 1200));

  const consent = await hPage.evaluate(() => {
    const card = document.querySelector('.present-consent');
    if (!card) return null;
    return {
      title: card.querySelector('h3')?.textContent?.trim(),
      requested_by: card.querySelector('.mono')?.textContent?.trim(),
      disclosure_pill: card.querySelector('.pill')?.textContent?.trim(),
      claim_rows: [...card.querySelectorAll('dl.cred-rows dt')].map((dt, i) => ({
        claim: dt.textContent.trim(),
        value: dt.nextElementSibling?.textContent?.trim(),
      })),
      disclose_present: !!card.querySelector('button[type="submit"]:not(.ghost)'),
      decline_present: !!card.querySelector('button.ghost'),
    };
  });
  console.log('[holder] consent card:');
  console.log(JSON.stringify(consent, null, 2));

  await hPage.close();
  await hCtx.close();
} finally {
  await br.close();
}
