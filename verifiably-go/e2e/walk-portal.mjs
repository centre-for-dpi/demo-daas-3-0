import puppeteer from 'puppeteer-core';
const browser = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const page = await browser.newPage();
const calls = [];
page.on('response', async r => { if (/api/.test(r.url()) && !/\.(png|svg|js|css)$/.test(r.url())) calls.push({m:r.request().method(), s:r.status(), u:r.url(), t: await r.text().catch(()=>''). then ? '' : ''}); });
page.on('request', r => { if (/api/.test(r.url()) && (r.method()!=='GET')) console.log(r.method(), r.url(), '—', (r.postData()||'').slice(0,400)); });
await page.goto('https://portal.walt.id/', { waitUntil: 'networkidle2', timeout: 30000 });
await new Promise(r => setTimeout(r, 2500));
// Try: find the "Try for free" / "Issue" flow
await page.evaluate(() => {
  const c = [...document.querySelectorAll('a, button')].find(e => /issue/i.test(e.textContent));
  c?.click();
});
await new Promise(r => setTimeout(r, 2500));
console.log('URL after Issue click:', page.url());
// Dump links
const links = await page.$$eval('a', a => a.map(x => x.href).filter(h => /portal/.test(h)).slice(0, 15));
console.log('local links:', links);
// Try clicking an OpenBadge tile
await page.evaluate(() => {
  const t = [...document.querySelectorAll('*')].find(e => /openbadge/i.test(e.textContent) && e.tagName !== 'HTML' && e.tagName !== 'BODY');
  const card = t?.closest('[class*="card"], [class*="Card"], button, a');
  (card||t)?.click();
});
await new Promise(r => setTimeout(r, 3000));
console.log('URL after OpenBadge click:', page.url());
// Find format chips/toggles
const chips = await page.evaluate(() => {
  const mat = [];
  for (const e of document.querySelectorAll('button, [role="tab"], [role="radio"], [class*="chip" i]')) {
    const t = (e.textContent||'').trim();
    if (t && t.length < 25) mat.push(t);
  }
  return [...new Set(mat)].slice(0, 40);
});
console.log('clickable shortish texts:', chips);
// Try clicking button labeled with a credential
await page.evaluate(() => {
  const c = [...document.querySelectorAll('*')].find(e => /open\s*badge/i.test(e.textContent||'') && e.children.length < 5);
  const up = c?.closest('button, a, [role="button"], [class*="card" i]') || c;
  up?.click();
});
await new Promise(r => setTimeout(r, 3000));
console.log('URL after second click:', page.url());
const after = await page.evaluate(() => document.body.innerText.slice(0, 1500));
console.log('text head:\n', after);
await browser.close();
