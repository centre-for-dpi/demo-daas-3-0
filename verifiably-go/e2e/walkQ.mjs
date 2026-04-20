// Verify: picking the "LDP · W3C" chip on the verifier side produces a PD
// asking for ldp_vc specifically, not jwt_vc_json. Previously the Std
// (w3c_vcdm_2) hardcoded jwt_vc_json on the wire and LDP chip selection
// silently dropped.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
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
  await new Promise(r => setTimeout(r, 600));
}
try {
  const ctx = await br.createBrowserContext();
  const p = await ctx.newPage();
  await p.setViewport({ width: 1500, height: 1100 });
  await auth(p, 'verifier');
  await p.goto('http://localhost:8080/verifier/dpg', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 400));
  const card = await p.$('.dpg-card[data-vendor="Walt Community Stack"]');
  if (card) { await card.click(); await new Promise(r => setTimeout(r, 400)); await p.click('#verifier-dpg-continue').catch(()=>{}); await new Promise(r => setTimeout(r, 900)); }
  await p.goto('http://localhost:8080/verifier/verify', { waitUntil: 'networkidle2' });
  await p.waitForSelector('#verifier-custom-body .schema-card', { timeout: 15000 });

  // Pick University Degree → LDP · W3C chip
  await p.evaluate(() => {
    const c = [...document.querySelectorAll('#verifier-custom-body .schema-card')].find(x => x.dataset.name === 'University Degree');
    const chip = [...c.querySelectorAll('.chip.small')].find(x => x.title === 'ldp_vc');
    chip?.click();
  });
  await p.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 500));

  // Generate
  await p.evaluate(() => {
    const b = [...document.querySelectorAll('#custom-template-form button[type="submit"]')].find(x => /Generate/i.test(x.textContent||''));
    b?.click();
  });
  await p.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 800));

  const summaryText = await p.evaluate(() => document.querySelector('#oid4vp-output .cred-preview')?.innerText?.slice(0, 300));
  console.log('request summary:', summaryText);

  const uri = await p.evaluate(() => document.querySelector('.link-display')?.textContent?.trim());
  const pdURI = new URL(uri).searchParams.get('presentation_definition_uri');
  const pd = await (await fetch(pdURI)).json();
  const d = pd.input_descriptors?.[0];
  console.log('\nPD format map:', JSON.stringify(d.format));
  console.log('PD field paths:', d.constraints?.fields?.map(f => f.path?.[0]));
  await p.close();
  await ctx.close();
} finally { await br.close(); }
