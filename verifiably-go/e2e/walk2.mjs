import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const page = await br.newPage();
const issueCalls = [];
page.on('request', req => {
  const m = req.method();
  if (m === 'GET' || !/api|walt/.test(req.url())) return;
  issueCalls.push({ m, u: req.url(), body: (req.postData()||'').slice(0, 800) });
});
page.on('response', async r => {
  if (!issueCalls.length) return;
  const last = issueCalls[issueCalls.length-1];
  if (last.u === r.url() && !last.status) { last.status = r.status(); last.resp = (await r.text().catch(()=>'')).slice(0, 600); }
});
await page.goto('https://portal.walt.id/', { waitUntil: 'networkidle2' });
await new Promise(r => setTimeout(r, 2000));
// Click OpenBadge tile (IdentityCredential also fine)
await page.evaluate(() => {
  const els = [...document.querySelectorAll('*')];
  const target = els.find(e => (e.textContent||'').trim() === 'OpenBadgeCredential');
  const up = target?.closest('a,button,[role="button"],[class*="card" i]') || target;
  up?.click();
});
await page.waitForNetworkIdle({ idleTime: 1000, timeout: 8000 }).catch(()=>{});
console.log('after click, URL:', page.url());
const text = await page.evaluate(() => document.body.innerText.slice(0, 2000));
console.log('TEXT:\n', text);
// Look for all active form inputs  
const inputs = await page.$$eval('input,select,textarea', x => x.map(i => ({t: i.tagName, type: i.type, name: i.name, placeholder: i.placeholder, value: i.value.slice(0,60)})));
console.log('INPUTS:', JSON.stringify(inputs, null, 2).slice(0, 2000));
// Buttons / chips / toggles containing format-like keywords
const chips = await page.evaluate(() => [...document.querySelectorAll('button,[role="tab"],[role="radio"],[class*="chip" i],[class*="toggle" i],[class*="select" i] label, li')]
  .map(e => ({ tag: e.tagName, text: (e.textContent||'').trim().slice(0,60) })).filter(x => x.text).slice(0,60));
console.log('CHIPS:', JSON.stringify(chips, null, 2));
await br.close();
console.log('\nAPI calls during click:');
for (const c of issueCalls) console.log(c.m, c.u, '→', c.status);
