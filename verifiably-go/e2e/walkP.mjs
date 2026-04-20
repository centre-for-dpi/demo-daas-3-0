// Full selective-disclosure round-trip via the UI:
//  1. Issuer UI: issue an Open Badge Credential in vc+sd-jwt (via selectiveDisclosure)
//  2. Holder: accept, pick the fresh SD-JWT cred
//  3. Verifier UI: build request for holder + achievement (NOT issuedOn)
//  4. Holder: consent card, Disclose
//  5. Verifier: inspect the vp_token — should have only the requested disclosures
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

async function runIssuer() {
  const ctx = await br.createBrowserContext();
  const p = await ctx.newPage();
  await p.setViewport({ width: 1400, height: 1100 });
  await auth(p, 'issuer');
  await pickDPG(p, 'issuer');
  await p.goto('http://localhost:8080/issuer/schema', { waitUntil: 'networkidle2' });
  await p.waitForSelector('.schema-card', { timeout: 15000 });
  await new Promise(r => setTimeout(r, 400));
  // Pick Open Badge Credential → vc+sd-jwt chip
  await p.evaluate(() => {
    const c = [...document.querySelectorAll('.schema-card')].find(x => x.dataset.name === 'Open Badge Credential');
    const chip = [...c.querySelectorAll('.chip.small')].find(x => x.title === 'vc+sd-jwt');
    chip?.click();
  });
  await p.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 400));
  // Continue to mode → single + wallet → issue
  await p.goto('http://localhost:8080/issuer/mode', { waitUntil: 'networkidle2' });
  await p.evaluate(() => document.querySelector('button[hx-vals*="single"]')?.click());
  await new Promise(r => setTimeout(r, 300));
  await p.evaluate(() => document.querySelector('button[hx-vals*="wallet"]')?.click());
  await new Promise(r => setTimeout(r, 300));
  await p.goto('http://localhost:8080/issuer/issue', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 600));
  // Fill fields
  await p.evaluate(() => {
    for (const [name, val] of [['holder', 'jo-sdjwt'], ['achievement', 'excellent'], ['issuedOn', '2026-04-20']]) {
      const i = document.querySelector(`input[name="field_${name}"]`);
      if (i) { i.value = val; i.dispatchEvent(new Event('input', {bubbles:true})); }
    }
  });
  await p.evaluate(() => {
    const b = [...document.querySelectorAll('button')].find(x => /Issue credential/i.test(x.textContent||''));
    b?.click();
  });
  await p.waitForNetworkIdle({ idleTime: 600, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 1200));
  const offerURI = await p.evaluate(() => document.querySelector('.link-display')?.textContent?.trim());
  console.log('[issuer] offer:', offerURI?.slice(0, 90) + '…');
  await p.close();
  await ctx.close();
  return offerURI;
}

async function claimInWallet(offerURI) {
  const ctx = await br.createBrowserContext();
  const p = await ctx.newPage();
  await p.setViewport({ width: 1400, height: 1100 });
  await auth(p, 'holder');
  await pickDPG(p, 'holder');
  await p.goto('http://localhost:8080/holder/wallet', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 400));
  // Paste offer
  await p.evaluate((uri) => {
    const ta = document.getElementById('offer-paste');
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
    setter.call(ta, uri);
    ta.dispatchEvent(new Event('input', {bubbles:true}));
    ta.closest('form').requestSubmit();
  }, offerURI);
  await p.waitForNetworkIdle({ idleTime: 600, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 1000));
  // Accept pending offer
  await p.evaluate(() => {
    const b = [...document.querySelectorAll('button')].find(x => /Accept/i.test((x.textContent||'').trim()));
    b?.click();
  });
  await p.waitForNetworkIdle({ idleTime: 600, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 1000));
  console.log('[holder] offer accepted');
  await p.close();
  return ctx;
}

async function verifierGenerate(wanted) {
  const ctx = await br.createBrowserContext();
  const p = await ctx.newPage();
  await p.setViewport({ width: 1500, height: 1100 });
  await auth(p, 'verifier');
  await pickDPG(p, 'verifier');
  await p.goto('http://localhost:8080/verifier/verify', { waitUntil: 'networkidle2' });
  await p.waitForSelector('#verifier-custom-body .schema-card', { timeout: 15000 });
  await new Promise(r => setTimeout(r, 400));
  // Pick Open Badge Credential → vc+sd-jwt
  await p.evaluate(() => {
    const c = [...document.querySelectorAll('#verifier-custom-body .schema-card')].find(x => x.dataset.name === 'Open Badge Credential');
    const chip = [...c.querySelectorAll('.chip.small')].find(x => x.title === 'vc+sd-jwt');
    chip?.click();
  });
  await p.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 400));
  // Uncheck fields NOT in `wanted`
  await p.evaluate((keep) => {
    for (const cb of document.querySelectorAll('input[name="field_key"]')) {
      if (!keep.includes(cb.value) && cb.checked) cb.click();
    }
  }, wanted);
  await new Promise(r => setTimeout(r, 200));
  await p.evaluate(() => {
    const b = [...document.querySelectorAll('#custom-template-form button[type="submit"]')].find(x => /Generate/i.test(x.textContent||''));
    b?.click();
  });
  await p.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 1000));
  const uri = await p.evaluate(() => document.querySelector('.link-display')?.textContent?.trim());
  const state = new URL(uri).searchParams.get('state');
  await p.close();
  await ctx.close();
  return { uri, state };
}

async function holderPresent(offerCtx, reqURI) {
  const p = await offerCtx.newPage();
  await p.setViewport({ width: 1400, height: 1100 });
  await p.goto('http://localhost:8080/holder/present', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 400));
  await p.evaluate((uri) => {
    const ta = document.querySelector('textarea[name="request_uri"]');
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
    setter.call(ta, uri);
    ta.dispatchEvent(new Event('input', {bubbles:true}));
  }, reqURI);
  // Pick the most-recent Open Badge in vc+sd-jwt format (last matching option).
  const dropdownState = await p.evaluate(() => {
    const sel = document.querySelector('select[name="credential_id"]');
    const opts = [...sel.options];
    const match = [...opts].reverse().find(o => /Open Badge/i.test(o.textContent) && o.dataset.format === 'vc+sd-jwt');
    if (match) sel.value = match.value;
    return { picked: sel.options[sel.selectedIndex]?.textContent?.trim(), total: opts.length };
  });
  console.log('[holder] dropdown:', dropdownState);
  await p.evaluate(() => {
    const b = [...document.querySelectorAll('button[type="submit"]')].find(x => /Review/i.test(x.textContent||''));
    b?.click();
  });
  await p.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 1000));
  const compat = await p.evaluate(() => !document.querySelector('.present-consent.incompatible'));
  console.log('[holder] consent compatible:', compat);
  if (!compat) {
    const reason = await p.evaluate(() => document.querySelector('.present-consent-block')?.innerText);
    console.log('[holder] incompatible reason:', reason);
    await p.close();
    return false;
  }
  // Click Disclose
  await p.evaluate(() => {
    const b = [...document.querySelectorAll('.present-consent button[type="submit"]')].find(x => /Disclose/i.test(x.textContent||''));
    b?.click();
  });
  await p.waitForNetworkIdle({ idleTime: 600, timeout: 15000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 1500));
  const result = await p.evaluate(() => document.getElementById('present-result')?.innerText?.slice(0, 200));
  console.log('[holder] submit result:', result);
  await p.close();
  return true;
}

try {
  const offer = await runIssuer();
  if (!offer) { console.log('no offer, abort'); process.exit(1); }
  const holderCtx = await claimInWallet(offer);
  const { uri, state } = await verifierGenerate(['holder', 'achievement']);
  console.log('[verifier] state:', state);
  const ok = await holderPresent(holderCtx, uri);
  if (!ok) { await holderCtx.close(); process.exit(1); }

  // Poll session + inspect vp_token
  await new Promise(r => setTimeout(r, 1500));
  const sess = await (await fetch(`http://localhost:7003/openid4vc/session/${state}`)).json();
  const vpt = typeof sess.tokenResponse?.vp_token === 'string' ? sess.tokenResponse.vp_token : null;
  if (!vpt) { console.log('no vp_token'); await holderCtx.close(); process.exit(1); }
  const parts = vpt.split('~').filter(Boolean);
  console.log(`vp_token: ${parts.length} segments`);
  for (let i = 1; i < parts.length; i++) {
    const p = parts[i];
    const pad = '='.repeat((4 - p.length % 4) % 4);
    try {
      const dec = Buffer.from(p + pad, 'base64url').toString();
      if (dec.startsWith('[')) {
        const arr = JSON.parse(dec);
        console.log(`  disclosure[${arr[1]}]: ${JSON.stringify(arr[2])}`);
      }
    } catch {}
  }
  console.log('overallVerificationResult:', sess.overallVerificationResult);
  await holderCtx.close();
} finally {
  await br.close();
}
