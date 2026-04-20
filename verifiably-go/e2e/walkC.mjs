// Expand every clickable heading/accordion + scroll to reveal the format toggle.

import puppeteer from 'puppeteer-core';

const browser = await puppeteer.launch({
  executablePath: '/usr/bin/google-chrome', headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
const page = await browser.newPage();
await page.setViewport({ width: 1600, height: 1800 });

await page.goto('https://portal.walt.id/credentials?ids=IdentityCredential', { waitUntil: 'networkidle2' });
await new Promise((r) => setTimeout(r, 3500));

// Before expansion: initial button count
console.log('initial dropdowns:', await page.$$eval('button[aria-haspopup]', (xs) => xs.length));

// Expand every H3 and its immediate sibling
await page.evaluate(() => {
  [...document.querySelectorAll('h3, h4, [class*="accordion" i], [class*="collaps" i]')].forEach((e) => e.click());
});
await new Promise((r) => setTimeout(r, 500));
await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
await new Promise((r) => setTimeout(r, 500));

console.log('after expand, dropdowns:', await page.$$eval('button[aria-haspopup]', (xs) => xs.length));

// Dump body text
const txt = await page.evaluate(() => document.body.innerText);
console.log('\n---- body text ----');
console.log(txt);

// All dropdowns + their options
const dropdowns = await page.evaluate(() => {
  return [...document.querySelectorAll('button[aria-haspopup]')].map((b, i) => {
    const parent = b.closest('div')?.parentElement;
    return { idx: i, text: (b.textContent || '').trim(), near: (parent?.textContent || '').trim().slice(0, 200) };
  });
});
console.log('\n---- dropdowns ----');
dropdowns.forEach((d) => console.log(` [${d.idx}] "${d.text}"   near: ${d.near}`));

for (let i = 0; i < dropdowns.length; i += 1) {
  await page.evaluate((idx) => [...document.querySelectorAll('button[aria-haspopup]')][idx]?.click(), i);
  await new Promise((r) => setTimeout(r, 500));
  const opts = await page.evaluate(() => [...document.querySelectorAll('[role="option"], [role="menuitem"]')].map((e) => (e.textContent || '').trim()).filter(Boolean));
  console.log(` dropdown #${i} ${dropdowns[i].text} options:`, JSON.stringify(opts));
  await page.keyboard.press('Escape');
  await new Promise((r) => setTimeout(r, 300));
}

// Try to find any button that contains the exact format-label strings
const hits = await page.evaluate(() => {
  const labels = ['JWT + W3C VC', 'SD-JWT + W3C VC', 'SD-JWT + IETF SD-JWT VC'];
  const out = [];
  for (const el of document.querySelectorAll('*')) {
    const t = (el.textContent || '').trim();
    for (const l of labels) {
      if (t === l || (t.length < l.length + 5 && t.includes(l))) {
        out.push({ tag: el.tagName, text: t, role: el.getAttribute('role') || '', html: el.outerHTML.slice(0, 300) });
      }
    }
  }
  return out.slice(0, 20);
});
console.log('\n---- elements matching format labels ----');
hits.forEach((h) => console.log(h));

await page.screenshot({ path: '/tmp/walkC-expanded.png', fullPage: true });

await browser.close();
