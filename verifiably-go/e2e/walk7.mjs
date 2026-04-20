import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const page = await br.newPage();
await page.setViewport({ width: 1600, height: 1200 });
const calls = [];
page.on('request', req => {
  if (req.method() === 'GET' && !req.url().includes('/api/')) return;
  calls.push({ m: req.method(), u: req.url(), body: (req.postData()||'').slice(0, 1500) });
});
page.on('response', async r => {
  const c = calls.find(x => x.u === r.url() && !x.status);
  if (c) { c.status = r.status(); try { c.resp = (await r.text()).slice(0, 1200); } catch {} }
});
await page.goto('https://portal.walt.id/', { waitUntil: 'networkidle2' });
await new Promise(r => setTimeout(r, 2500));
await page.evaluate(() => {
  const h6 = [...document.querySelectorAll('h6')].find(e => (e.textContent||'').trim() === 'OpenBadgeCredential');
  const card = h6?.closest('div.drop-shadow-sm') || h6?.closest('div');
  card?.click();
});
await new Promise(r => setTimeout(r, 800));
await page.evaluate(() => {
  const b = [...document.querySelectorAll('button')].find(e => /^\s*start\s*$/i.test(e.textContent||''));
  b?.click();
});
await page.waitForNetworkIdle({ idleTime: 1500, timeout: 8000 }).catch(()=>{});
await new Promise(r => setTimeout(r, 3000));
console.log('URL:', page.url());
// Expand "Credential Configuration" — likely contains format toggle
await page.evaluate(() => {
  const els = [...document.querySelectorAll('h2, h3, h4, h5, button, [role="button"]')];
  const t = els.find(e => /Credential Configuration/i.test(e.textContent||''));
  t?.click();
});
await new Promise(r => setTimeout(r, 1500));
// Also try clicking any dropdowns/selects to reveal their options
const selectButtons = await page.$$('button[aria-haspopup], button[role="combobox"]');
console.log('dropdown buttons found:', selectButtons.length);
for (const sb of selectButtons.slice(0, 5)) {
  await sb.click().catch(()=>{});
  await new Promise(r => setTimeout(r, 400));
}
await page.screenshot({ path: '/tmp/portal-step2-expanded.png', fullPage: true });
console.log('TEXT:\n', (await page.evaluate(()=>document.body.innerText)).slice(0, 4500));
// Hunt for visible option-labels (SdJwt, W3C, JWT VC, vc+sd-jwt, etc.)
const opts = await page.evaluate(() => [...document.querySelectorAll('[role="option"], [role="menuitem"], option')]
  .map(e => (e.textContent||'').trim()).filter(Boolean).slice(0, 40));
console.log('\nROLE=option texts:', JSON.stringify(opts));
// Click Issue to see what issuance call gets made
await page.evaluate(() => {
  const b = [...document.querySelectorAll('button')].find(e => /^\s*issue\s*$/i.test(e.textContent||''));
  b?.click();
});
await page.waitForNetworkIdle({ idleTime: 1500, timeout: 15000 }).catch(()=>{});
console.log('\nURL after Issue:', page.url());
console.log('calls with bodies:');
for (const c of calls.filter(x => x.m !== 'GET' || x.u.includes('issue'))) {
  console.log(c.m, c.u, '→', c.status||'');
  if (c.body) console.log('  body:', c.body.slice(0, 400));
  if (c.resp) console.log('  resp:', c.resp.slice(0, 300));
}
await br.close();
