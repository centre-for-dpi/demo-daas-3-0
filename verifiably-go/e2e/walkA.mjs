// Click walt.id's format selector using the exact XPath the user provided:
//   /html/body/div/div/div[3]/div[2]/div/div[2]/div[2]/div/button
// Then enumerate the options that pop up.

import puppeteer from 'puppeteer-core';

const browser = await puppeteer.launch({
  executablePath: '/usr/bin/google-chrome', headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
const page = await browser.newPage();
await page.setViewport({ width: 1600, height: 1400 });

const calls = [];
page.on('request', (req) => {
  if (!/walt\.id\/api|credentials\.walt\.id/.test(req.url())) return;
  calls.push({ m: req.method(), u: req.url(), body: (req.postData() || '').slice(0, 2500) });
});
page.on('response', async (r) => {
  const c = calls.find((x) => x.u === r.url() && !x.status);
  if (c) { c.status = r.status(); try { c.resp = (await r.text()).slice(0, 2000); } catch {} }
});

await page.goto('https://portal.walt.id/', { waitUntil: 'networkidle2' });
await new Promise((r) => setTimeout(r, 2500));

// Pick IdentityCredential — simulate a real user click at the tile's center coordinate
const box = await page.evaluate(() => {
  const h6 = [...document.querySelectorAll('h6')].find((e) => (e.textContent || '').trim() === 'IdentityCredential');
  const card = h6?.closest('div.drop-shadow-sm') || h6?.closest('div');
  if (!card) return null;
  const r = card.getBoundingClientRect();
  return { x: r.x + r.width / 2, y: r.y + r.height / 2 };
});
if (box) {
  await page.mouse.click(box.x, box.y);
  await new Promise((r) => setTimeout(r, 400));
  // click again in case first toggled off? check selected state
  const selected = await page.evaluate(() => {
    const h6 = [...document.querySelectorAll('h6')].find((e) => (e.textContent || '').trim() === 'IdentityCredential');
    const card = h6?.closest('div.drop-shadow-sm') || h6?.closest('div');
    return card && /ring|selected|active|from-primary-600|border-/.test(card.className);
  });
  console.log('tile selected?', selected);
}
await new Promise((r) => setTimeout(r, 600));
await page.evaluate(() => {
  const b = [...document.querySelectorAll('button')].find((e) => /^\s*start\s*$/i.test(e.textContent || ''));
  b?.click();
});
await page.waitForNetworkIdle({ idleTime: 1200, timeout: 10000 }).catch(() => {});
await new Promise((r) => setTimeout(r, 4000));
console.log('URL:', page.url());
await page.screenshot({ path: '/tmp/walkA-after-start.png', fullPage: true });

// Use user-provided XPath to click the format selector button.
const xpath = '/html/body/div/div/div[3]/div[2]/div/div[2]/div[2]/div/button';
const handle = await page.evaluateHandle((xp) => {
  const r = document.evaluate(xp, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null);
  return r.singleNodeValue;
}, xpath);

const tagInfo = await page.evaluate((el) => el ? {
  tag: el.tagName,
  text: (el.textContent || '').trim().slice(0, 120),
  aria: el.getAttribute('aria-haspopup') || '',
  hasPopup: !!el.getAttribute('aria-haspopup'),
  html: el.outerHTML.slice(0, 500),
} : null, handle);
console.log('\n---- target button info ----');
console.log(tagInfo);

if (handle) {
  await handle.click();
  await new Promise((r) => setTimeout(r, 800));
}

await page.screenshot({ path: '/tmp/walkA-dropdown-open.png', fullPage: true });

const opts = await page.evaluate(() => {
  const roles = [...document.querySelectorAll('[role="option"], [role="menuitem"]')]
    .map((e) => ({ text: (e.textContent || '').trim(), role: e.getAttribute('role') }))
    .filter((x) => x.text);
  return roles.slice(0, 30);
});
console.log('\n---- popped options ----');
opts.forEach((o) => console.log(' •', o.role, ':', o.text));

// Click each option and watch for network reactions / form mutations.
console.log('\n---- for each format option, click it and snapshot form ----');
const uniqueTexts = [...new Set(opts.map((o) => o.text))];
for (const label of uniqueTexts.slice(0, 5)) {
  // Re-open the dropdown (XPath may have moved)
  const h2 = await page.evaluateHandle((xp) => {
    const r = document.evaluate(xp, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null);
    return r.singleNodeValue;
  }, xpath);
  await h2?.click();
  await new Promise((r) => setTimeout(r, 500));
  // Click the option whose text matches `label`
  const picked = await page.evaluate((lbl) => {
    const o = [...document.querySelectorAll('[role="option"], [role="menuitem"]')].find((e) => (e.textContent || '').trim() === lbl);
    o?.click();
    return !!o;
  }, label);
  await new Promise((r) => setTimeout(r, 800));
  if (!picked) { console.log(` [${label}] (could not find)`); continue; }

  // What does the form look like now?
  const snapshot = await page.evaluate(() => {
    const headings = [...document.querySelectorAll('h1,h2,h3,h4,h5,h6,label')]
      .map((e) => `${e.tagName}: ${(e.textContent || '').trim()}`).filter((t) => t.length < 120).slice(0, 40);
    const inputs = [...document.querySelectorAll('input, textarea')]
      .filter((i) => !['hidden', 'submit', 'button'].includes(i.type))
      .map((i) => ({ name: i.name || i.id, type: i.type, placeholder: i.placeholder, value: (i.value || '').slice(0, 40) }));
    const buttonText = [...document.querySelectorAll('button[aria-haspopup]')]
      .map((b) => (b.textContent || '').trim());
    return { headings, inputs, buttonText };
  });
  console.log(`\n>>> option "${label}" selected`);
  console.log('  aria-haspopup buttons now:', snapshot.buttonText.join(' | '));
  console.log('  fields (' + snapshot.inputs.length + '):',
    snapshot.inputs.slice(0, 15).map((i) => `${i.name}:${i.type}`).join(', '));
}

// Now click "Issue" and observe the actual POST body — tells us what the backend expects per format
console.log('\n---- click Issue and snapshot issuance POST ----');
const reqPromise = page.waitForResponse((r) => /\/(openid4vc|credentials)\/(issue|offer)/i.test(r.url()) && r.request().method() === 'POST', { timeout: 15000 }).catch(() => null);
await page.evaluate(() => {
  const b = [...document.querySelectorAll('button')].find((e) => /^\s*issue\s*$/i.test(e.textContent || ''));
  b?.click();
});
const resp = await reqPromise;
if (resp) {
  console.log('Issue POST URL:', resp.url(), '→', resp.status());
  console.log('Issue POST body:', resp.request().postData()?.slice(0, 2500) || '(none)');
  try { console.log('Issue POST resp:', (await resp.text()).slice(0, 500)); } catch {}
}

console.log('\n---- all API calls observed ----');
for (const c of calls) {
  console.log(` ${c.status || '---'} ${c.m} ${c.u}`);
  if (c.body && c.m === 'POST') console.log('     body:', c.body.slice(0, 800));
}

await browser.close();
