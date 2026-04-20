// Verify full-disclosure SD-JWT presentation round-trip goes through
// cleanly (no "Field 'input_descriptors' is required" error). Use the
// walt.id API directly to isolate the verifier body we generate.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const p = await br.newPage();
await p.setViewport({ width: 1500, height: 1200 });

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
  if (c) { await c.click(); await new Promise(r => setTimeout(r, 300)); await page.click(`#${role}-dpg-continue`).catch(()=>{}); await new Promise(r => setTimeout(r, 800)); }
}

try {
  // Verifier with FULL disclosure
  await auth(p, 'verifier');
  await pickDPG(p, 'verifier');
  await p.goto('http://localhost:8080/verifier/verify', { waitUntil: 'networkidle2' });
  await p.waitForSelector('#verifier-custom-body .schema-card', { timeout: 15000 });
  await p.evaluate(() => {
    const c = [...document.querySelectorAll('#verifier-custom-body .schema-card')].find(x => x.dataset.name === 'University Degree');
    const chip = [...c.querySelectorAll('.chip.small')].find(x => x.title === 'vc+sd-jwt');
    chip?.click();
  });
  await p.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 400));
  // Switch to FULL disclosure
  await p.evaluate(() => {
    const r = document.querySelector('input[name="disclosure"][value="full"]');
    if (r) { r.checked = true; r.dispatchEvent(new Event('change', {bubbles:true})); }
  });
  await new Promise(r => setTimeout(r, 150));
  await p.evaluate(() => {
    const b = [...document.querySelectorAll('#custom-template-form button[type="submit"]')].find(x => /Generate/i.test(x.textContent||''));
    b?.click();
  });
  await p.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 800));
  const uri = await p.evaluate(() => document.querySelector('.link-display')?.textContent?.trim());
  const pdURI = new URL(uri).searchParams.get('presentation_definition_uri');
  const pd = await (await fetch(pdURI)).json();
  const d = pd.input_descriptors?.[0];
  console.log('full-disclosure PD:');
  console.log('  format:', JSON.stringify(d?.format));
  console.log('  limit_disclosure:', d?.constraints?.limit_disclosure);
  console.log('  field paths:', d?.constraints?.fields?.map(f => f.path?.[0]));
  console.log('  has input_descriptors:', !!pd.input_descriptors);
} finally { await br.close(); }
