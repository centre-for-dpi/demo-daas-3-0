import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const page = await br.newPage();
await page.goto('https://portal.walt.id/', { waitUntil: 'networkidle2' });
await new Promise(r => setTimeout(r, 2000));
// Get the OpenBadge tile's actual outerHTML
const html = await page.evaluate(() => {
  const target = [...document.querySelectorAll('*')].find(e => (e.textContent||'').trim() === 'OpenBadgeCredential');
  if (!target) return 'not found';
  let el = target;
  for (let i = 0; i < 5; i++) {
    if (el.parentElement && el.parentElement.children.length > 5) return el.parentElement.outerHTML.slice(0, 1500);
    el = el.parentElement;
    if (!el) break;
  }
  return target.parentElement?.outerHTML.slice(0,1500);
});
console.log('=== TILE HTML ===');
console.log(html);
console.log();
// Also: try pressing on a tile via coords
const box = await page.evaluate(() => {
  const target = [...document.querySelectorAll('*')].find(e => (e.textContent||'').trim() === 'OpenBadgeCredential');
  const r = target?.getBoundingClientRect();
  return r ? { x: r.x+10, y: r.y+10, width: r.width, height: r.height } : null;
});
console.log('box:', box);
if (box && box.width > 0) {
  await page.mouse.click(box.x, box.y);
  await new Promise(r => setTimeout(r, 1500));
  // Did a selection dot appear?
  const state = await page.evaluate(() => {
    const target = [...document.querySelectorAll('*')].find(e => (e.textContent||'').trim() === 'OpenBadgeCredential');
    return { className: target?.parentElement?.className, outerHTML: target?.parentElement?.outerHTML.slice(0, 400) };
  });
  console.log('after click:', state);
  // Click Start
  await page.evaluate(() => {
    const b = [...document.querySelectorAll('button')].find(e => /^\s*start\s*$/i.test(e.textContent||''));
    b?.click();
  });
  await new Promise(r => setTimeout(r, 3000));
  console.log('\nAfter Start URL:', page.url());
  const text = await page.evaluate(()=>document.body.innerText.slice(0,3000));
  console.log('TEXT:\n', text);
  // See buttons
  const btns = await page.$$eval('button,[role="tab"],[role="radio"],[role="option"],input[type="radio"]', xs=>xs.map(x=>((x.textContent||x.value||'').trim().slice(0,50))).filter(Boolean));
  console.log('\nBUTTONS:', btns.slice(0,40).join(' | '));
}
await br.close();
