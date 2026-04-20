import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const page = await br.newPage();
await page.setViewport({ width: 1600, height: 1200 });
const calls = [];
page.on('request', req => {
  if (req.method() === 'GET') return;
  calls.push({ m: req.method(), u: req.url(), body: (req.postData()||'').slice(0,1000) });
});
page.on('response', async r => {
  const match = calls.find(c => c.u === r.url() && !c.status);
  if (match) { match.status = r.status(); try { match.resp = (await r.text()).slice(0, 800); } catch {} }
});
await page.goto('https://portal.walt.id/', { waitUntil: 'networkidle2' });
await new Promise(r => setTimeout(r, 2500));
// Click the OpenBadge card (h6 inside)
await page.evaluate(() => {
  const h6 = [...document.querySelectorAll('h6')].find(e => (e.textContent||'').trim() === 'OpenBadgeCredential');
  const card = h6?.closest('div.drop-shadow-sm') || h6?.closest('div');
  card?.click();
});
await new Promise(r => setTimeout(r, 1500));
// Start
await page.evaluate(() => {
  const b = [...document.querySelectorAll('button')].find(e => /^\s*start\s*$/i.test(e.textContent||''));
  b?.click();
});
await page.waitForNetworkIdle({ idleTime: 1500, timeout: 10000 }).catch(()=>{});
await new Promise(r => setTimeout(r, 2500));
console.log('URL:', page.url());
console.log('TEXT:\n', (await page.evaluate(()=>document.body.innerText)).slice(0, 3500));
await page.screenshot({ path: '/tmp/portal-step1.png', fullPage: true });

// Look for format chips/toggles
const formats = await page.evaluate(() => {
  const all = [...document.querySelectorAll('button, [role="tab"], [role="option"], [role="radio"], [role="switch"], label')];
  return all.map(e => ({ text: (e.textContent||'').trim().slice(0,60), cls: (e.className||'').toString().slice(0,80) }))
    .filter(x => x.text && !['start','next','cancel'].includes(x.text.toLowerCase()));
});
console.log('\nbuttons/toggles:', JSON.stringify(formats, null, 2).slice(0, 3000));
console.log('\nNON-GET calls:');
for (const c of calls) console.log(c.m, c.u, '→', c.status||'', '|', c.body.slice(0,200));

// Walk to next step
await page.evaluate(() => {
  const b = [...document.querySelectorAll('button')].find(e => /^\s*next\s*$/i.test(e.textContent||''));
  b?.click();
});
await new Promise(r => setTimeout(r, 3000));
console.log('\nAfter Next URL:', page.url());
console.log('TEXT:\n', (await page.evaluate(()=>document.body.innerText)).slice(0, 3500));
await page.screenshot({ path: '/tmp/portal-step2.png', fullPage: true });

await br.close();
