import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const page = await br.newPage();
const calls = [];
page.on('request', req => {
  const m = req.method();
  if (m === 'GET' && /api/.test(req.url())) calls.push({ m, u: req.url() });
  if (m !== 'GET') calls.push({ m, u: req.url(), body: (req.postData()||'').slice(0,800) });
});
page.on('response', async r => {
  for (const c of calls) if (c.u === r.url() && !c.s) { c.s = r.status(); try { c.resp = (await r.text()).slice(0,500) } catch {} }
});

await page.goto('https://portal.walt.id/', { waitUntil: 'networkidle2' });
await new Promise(r => setTimeout(r, 1500));
// Select OpenBadge
await page.evaluate(() => {
  const t = [...document.querySelectorAll('*')].find(e => (e.textContent||'').trim() === 'OpenBadgeCredential');
  const card = t?.closest('[class*="card" i], [class*="item" i], li, div');
  card?.click();
});
await new Promise(r => setTimeout(r, 1000));
// Click Start
await page.evaluate(() => {
  const b = [...document.querySelectorAll('button')].find(e => /^\s*start\s*$/i.test(e.textContent||''));
  b?.click();
});
await new Promise(r => setTimeout(r, 3500));
console.log('=== URL after Start:', page.url());
console.log('=== TEXT (4000):\n', (await page.evaluate(()=>document.body.innerText)).slice(0, 4000));
console.log('\n=== INPUTS');
console.log(JSON.stringify(await page.$$eval('input,select,textarea,[role="combobox"]',xs=>xs.map(x=>({t:x.tagName,type:x.type,name:x.name,placeholder:x.placeholder,value:(x.value||'').slice(0,60),role:x.getAttribute('role')}))), null, 2).slice(0, 2500));
console.log('\n=== clickable text');
const btns = await page.$$eval('button,[role="tab"],[role="radio"],a', xs=>xs.map(x=>((x.textContent||'').trim().slice(0,50))).filter(Boolean));
console.log(btns.slice(0, 60).join(' | '));
console.log('\n=== API calls (post-Start)');
for (const c of calls.slice(-30)) console.log(c.m, c.u, '→', c.s||'');
await br.close();
