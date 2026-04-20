import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const page = await br.newPage();
await page.setViewport({ width: 1600, height: 1200 });
await page.goto('https://portal.walt.id/', { waitUntil: 'networkidle2' });
await new Promise(r => setTimeout(r, 3000));
await page.screenshot({ path: '/tmp/portal-landing.png', fullPage: true });
console.log('landing screenshot written');

// Dump broader DOM structure
const tree = await page.evaluate(() => {
  const topEls = [...document.querySelectorAll('main *')]
    .filter(e => e.children.length < 10 && (e.textContent||'').length < 200 && (e.textContent||'').length > 5)
    .slice(0, 40)
    .map(e => ({ tag: e.tagName, classes: (e.className||'').toString().slice(0,80), text: (e.textContent||'').trim().slice(0,80) }));
  return topEls;
});
console.log('DOM sample:');
for (const t of tree) console.log(` ${t.tag}  ${t.classes}  "${t.text}"`);

// Look for anything selectable
const cards = await page.evaluate(() => {
  return [...document.querySelectorAll('[class*="card" i], [class*="tile" i], [class*="item" i], [class*="box" i]')]
    .slice(0, 10)
    .map(e => ({ cls: e.className.toString().slice(0,100), children: e.children.length, text: (e.textContent||'').trim().slice(0,80) }));
});
console.log('\ncards:', JSON.stringify(cards, null, 2));

await br.close();
