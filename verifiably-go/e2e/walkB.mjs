// Robust: interact with React directly to set selectedIds, or use URL with hardcoded uuid.
// Portal API: https://credentials.walt.id/api/list returns an array — walk the list to get ids.

import puppeteer from 'puppeteer-core';

const browser = await puppeteer.launch({
  executablePath: '/usr/bin/google-chrome', headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
const page = await browser.newPage();
await page.setViewport({ width: 1600, height: 1400 });

// First fetch the list of credential ids
const list = await (await fetch('https://credentials.walt.id/api/list')).json();
console.log('first 5 entries:', list.slice(0, 5));
const identity = list.find((x) => x === 'IdentityCredential' || x.name === 'IdentityCredential' || x.id === 'IdentityCredential');
console.log('identity entry:', identity);

// Approach: go directly to /credentials?ids=IdentityCredential (walt.id's wizard uses name as id)
await page.goto('https://portal.walt.id/credentials?ids=IdentityCredential', { waitUntil: 'networkidle2' });
await new Promise((r) => setTimeout(r, 3500));
console.log('URL:', page.url());

// Take a snapshot of ALL buttons
const buttons = await page.evaluate(() => {
  return [...document.querySelectorAll('button')].map((b, i) => ({
    idx: i,
    text: (b.textContent || '').trim().slice(0, 80),
    haspopup: b.getAttribute('aria-haspopup') || '',
    ariaLabel: b.getAttribute('aria-label') || '',
  })).filter((b) => b.text || b.ariaLabel).slice(0, 50);
});
console.log('\n---- all buttons ----');
buttons.forEach((b, i) => console.log(`  [${b.idx}] popup=${b.haspopup} text="${b.text}"`));

// Grab every dropdown's current value and its full structure
const dropdowns = await page.evaluate(() => {
  return [...document.querySelectorAll('button[aria-haspopup]')].map((b, i) => ({
    idx: i,
    text: (b.textContent || '').trim(),
    parentText: (b.parentElement?.parentElement?.textContent || '').trim().slice(0, 150),
  }));
});
console.log('\n---- dropdown buttons ----');
dropdowns.forEach((d) => console.log(` [${d.idx}] "${d.text}"   (nearby: ${d.parentText})`));

// Take full-page screenshot
await page.screenshot({ path: '/tmp/walkB-wizard.png', fullPage: true });

// Click each dropdown and enumerate options
for (let i = 0; i < Math.min(dropdowns.length, 5); i += 1) {
  await page.evaluate((idx) => {
    [...document.querySelectorAll('button[aria-haspopup]')][idx]?.click();
  }, i);
  await new Promise((r) => setTimeout(r, 600));
  const opts = await page.evaluate(() => {
    return [...document.querySelectorAll('[role="option"], [role="menuitem"]')]
      .map((e) => (e.textContent || '').trim()).filter(Boolean);
  });
  console.log(`\ndropdown #${i} (${dropdowns[i].text}) options:`, JSON.stringify(opts));
  await page.keyboard.press('Escape');
  await new Promise((r) => setTimeout(r, 300));
}

// Find a field labeled / near "Format" — hunt for the exact label
const formatBlock = await page.evaluate(() => {
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  const hits = [];
  let n;
  while ((n = walker.nextNode())) {
    const t = (n.nodeValue || '').trim();
    if (/^(format|credential format)$/i.test(t)) {
      const el = n.parentElement;
      hits.push({ text: t, tag: el.tagName, html: el.parentElement?.outerHTML.slice(0, 800) });
    }
  }
  return hits;
});
console.log('\n---- "Format" label occurrences ----');
formatBlock.forEach((f) => console.log(f));

await browser.close();
