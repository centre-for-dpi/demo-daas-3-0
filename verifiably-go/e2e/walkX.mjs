// Replicate the "valid credential failed verification" flow: issue SD-JWT,
// present, then watch the verifier UI. Now that isTerminalSession requires
// an actual verdict flag, the auto-poll should settle on VALID.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox','--disable-dev-shm-usage'] });

async function auth(page, role) {
  await page.goto('http://localhost:8080/', { waitUntil: 'networkidle2' });
  await page.click(`button.role-card[value="${role}"]`);
  await page.waitForFunction(() => /\/auth/.test(location.pathname), { timeout: 10000 });
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(()=>null),
    page.evaluate(() => [...document.querySelectorAll('button.provider-btn')].find(b => (b.getAttribute('hx-vals')||'').includes('keycloak'))?.click()),
  ]);
  await page.waitForSelector('input[name="username"]', { timeout: 15000 });
  await page.type('input[name="username"]', 'admin');
  await page.type('input[name="password"]', 'admin');
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(()=>null),
    page.click('input[type="submit"], button[type="submit"]'),
  ]);
  await new Promise(r => setTimeout(r, 700));
}
async function pickDPG(page, role) {
  await page.goto(`http://localhost:8080/${role}/dpg`, { waitUntil: 'networkidle2' });
  const c = await page.$('.dpg-card[data-vendor="Walt Community Stack"]');
  if (c) { await c.click(); await new Promise(r => setTimeout(r, 300)); await page.click(`#${role}-dpg-continue`).catch(()=>{}); await new Promise(r => setTimeout(r, 900)); }
}

try {
  // Verifier tab stays open to watch polling result
  const vCtx = await br.createBrowserContext();
  const vPage = await vCtx.newPage();
  await vPage.setViewport({ width: 1500, height: 1100 });
  await auth(vPage, 'verifier');
  await pickDPG(vPage, 'verifier');
  await vPage.goto('http://localhost:8080/verifier/verify', { waitUntil: 'networkidle2' });
  await vPage.waitForSelector('#verifier-custom-body .schema-card', { timeout: 15000 });

  await vPage.evaluate(() => {
    const c = [...document.querySelectorAll('#verifier-custom-body .schema-card')].find(x => x.dataset.name === 'Open Badge Credential');
    const chip = [...c.querySelectorAll('.chip.small')].find(x => x.title === 'vc+sd-jwt');
    chip?.click();
  });
  await vPage.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 400));
  await vPage.evaluate(() => {
    const b = [...document.querySelectorAll('#custom-template-form button[type="submit"]')].find(x => /Generate/i.test(x.textContent||''));
    b?.click();
  });
  await vPage.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 800));
  const vURI = await vPage.evaluate(() => document.querySelector('#oid4vp-output .link-display')?.textContent?.trim());
  console.log('verifier URI generated');

  // Present via holder
  const hCtx = await br.createBrowserContext();
  const hPage = await hCtx.newPage();
  await hPage.setViewport({ width: 1400, height: 1100 });
  await auth(hPage, 'holder');
  await pickDPG(hPage, 'holder');
  await hPage.goto('http://localhost:8080/holder/present', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 400));
  await hPage.evaluate((u) => {
    const ta = document.querySelector('textarea[name="request_uri"]');
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
    setter.call(ta, u);
    ta.dispatchEvent(new Event('input', {bubbles:true}));
  }, vURI);
  await hPage.evaluate(() => {
    const sel = document.querySelector('select[name="credential_id"]');
    const m = [...sel.options].reverse().find(o => /Open Badge/i.test(o.textContent) && o.dataset.format === 'vc+sd-jwt');
    if (m) sel.value = m.value;
  });
  await hPage.evaluate(() => {
    [...document.querySelectorAll('button[type="submit"]')].find(x => /Review/i.test(x.textContent||''))?.click();
  });
  await hPage.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 800));
  await hPage.evaluate(() => {
    [...document.querySelectorAll('.present-consent button[type="submit"]')].find(x => /Disclose/i.test(x.textContent||''))?.click();
  });
  await hPage.waitForNetworkIdle({ idleTime: 600, timeout: 15000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 1500));
  console.log('holder disclosed');
  await hPage.close();
  await hCtx.close();

  // Wait for the verifier's auto-poll to pick up the terminal state.
  // With the isTerminalSession fix, we keep polling until verificationResult is set.
  for (let i = 0; i < 15; i++) {
    const st = await vPage.evaluate(() => {
      const flip = document.querySelector('.verify-flip');
      const pending = document.querySelector('.verify-result.pending');
      return {
        flipClass: flip ? (flip.classList.contains('valid') ? 'VALID' : flip.classList.contains('invalid') ? 'INVALID' : 'OTHER') : null,
        pending: !!pending,
      };
    });
    console.log(`  t=${(i+1)*2}s state:`, st);
    if (st.flipClass === 'VALID') { console.log('✓ settled on VALID'); break; }
    if (st.flipClass === 'INVALID') { console.log('✗ settled on INVALID — bug still present'); break; }
    await new Promise(r => setTimeout(r, 2000));
  }

  const final = await vPage.evaluate(() => {
    const flip = document.querySelector('.verify-flip');
    const back = flip?.querySelector('.verify-flip-back');
    return {
      valid: flip?.classList.contains('valid'),
      back_text: (back?.innerText || '').slice(0, 300),
    };
  });
  console.log('final:', final);
} finally { await br.close(); }
