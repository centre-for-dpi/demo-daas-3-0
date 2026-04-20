// Deep-dive the walt.id portal wizard to reveal:
//   1. How credentials are listed (one card per NAME or per NAME×format?)
//   2. Where the format toggle lives (sdjwtietf / sdjwtw3c / jwtw3c)
//   3. Whether the form fields change when format changes
//   4. What the /credentials/issue API body looks like per format
//
// Then dump the same observations for our local /issuer/schema so we can
// compare directly.

import puppeteer from 'puppeteer-core';

const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';
const PORTAL = 'https://portal.walt.id';

const browser = await puppeteer.launch({
  executablePath: CHROME, headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});

const page = await browser.newPage();
await page.setViewport({ width: 1600, height: 1200 });

const calls = [];
page.on('request', (req) => {
  const u = req.url();
  if (!/walt\.id\/api|credentials\.walt\.id/.test(u)) return;
  calls.push({ m: req.method(), u, body: (req.postData() || '').slice(0, 2000) });
});
page.on('response', async (r) => {
  const c = calls.find((x) => x.u === r.url() && !x.status);
  if (c) { c.status = r.status(); try { c.resp = (await r.text()).slice(0, 1500); } catch {} }
});

// 1) Landing — how many credential tiles, and what do they show?
await page.goto(PORTAL, { waitUntil: 'networkidle2' });
await new Promise((r) => setTimeout(r, 2500));

const tiles = await page.evaluate(() => {
  return [...document.querySelectorAll('h6')]
    .map((h) => {
      const card = h.closest('div.drop-shadow-sm') || h.closest('div');
      const name = (h.textContent || '').trim();
      // Look for format tags inside the card
      const tags = [...card.querySelectorAll('span, small, code, [class*="badge" i], [class*="pill" i], [class*="chip" i]')]
        .map((e) => (e.textContent || '').trim()).filter(Boolean);
      return { name, tags: tags.slice(0, 10) };
    });
});
console.log(`\n========== PORTAL LANDING (${tiles.length} tiles) ==========`);
tiles.slice(0, 15).forEach((t) => console.log(' •', t.name, '   tags:', t.tags.join('|') || '(none)'));

// 2) Click OpenBadgeCredential
await page.evaluate(() => {
  const h6 = [...document.querySelectorAll('h6')].find((e) => (e.textContent || '').trim() === 'OpenBadgeCredential');
  const card = h6?.closest('div.drop-shadow-sm') || h6?.closest('div');
  card?.click();
});
await new Promise((r) => setTimeout(r, 1500));

await page.screenshot({ path: '/tmp/walk8-after-tile-click.png', fullPage: true });

// See if the detail panel shows a format selector before Start
const preStartInfo = await page.evaluate(() => ({
  url: location.href,
  text: document.body.innerText.slice(0, 2000),
}));
console.log('\n---- after tile click ----');
console.log('URL:', preStartInfo.url);
console.log('TEXT:\n', preStartInfo.text.split('\n').filter(Boolean).slice(0, 30).join('\n'));

// 3) Click Start
await page.evaluate(() => {
  const b = [...document.querySelectorAll('button')].find((e) => /^\s*start\s*$/i.test(e.textContent || ''));
  b?.click();
});
await page.waitForNetworkIdle({ idleTime: 1200, timeout: 8000 }).catch(() => {});
await new Promise((r) => setTimeout(r, 2000));

console.log('\n---- after Start click ----');
console.log('URL:', page.url());

// 4) Enumerate EVERY button, select, dropdown, and their text — trying to find format toggle
const wizard = await page.evaluate(() => {
  const out = { headings: [], buttons: [], selects: [], inputs: [] };
  out.headings = [...document.querySelectorAll('h1, h2, h3, h4, h5, h6')]
    .map((e) => e.tagName + ': ' + (e.textContent || '').trim()).slice(0, 30);
  out.buttons = [...document.querySelectorAll('button')]
    .map((e) => ({
      text: (e.textContent || '').trim().slice(0, 60),
      role: e.getAttribute('role') || '',
      hasPopup: e.getAttribute('aria-haspopup') || '',
      ariaLabel: e.getAttribute('aria-label') || '',
    }))
    .filter((b) => b.text || b.ariaLabel)
    .slice(0, 40);
  out.selects = [...document.querySelectorAll('select')]
    .map((s) => ({
      name: s.name,
      value: s.value,
      options: [...s.options].map((o) => `${o.value}=${o.text}`).slice(0, 20),
    }));
  out.inputs = [...document.querySelectorAll('input, textarea')]
    .map((i) => ({ name: i.name || i.id || '', type: i.type, placeholder: i.placeholder || '', value: (i.value || '').slice(0, 40) }))
    .slice(0, 30);
  return out;
});
console.log('\n---- wizard DOM snapshot ----');
console.log('Headings:', wizard.headings);
console.log('Buttons:');
wizard.buttons.forEach((b) => console.log(' ·', JSON.stringify(b)));
console.log('Selects:');
wizard.selects.forEach((s) => console.log(' ·', JSON.stringify(s)));
console.log('Inputs:');
wizard.inputs.forEach((i) => console.log(' ·', JSON.stringify(i)));

// 5) Click every dropdown-ish button one at a time, snapshot its popped options, close
const popButtons = await page.$$('button[aria-haspopup], button[role="combobox"]');
console.log(`\n${popButtons.length} dropdown-ish buttons. Enumerating…`);
for (let i = 0; i < Math.min(popButtons.length, 8); i += 1) {
  await popButtons[i].click().catch(() => {});
  await new Promise((r) => setTimeout(r, 600));
  const popped = await page.evaluate(() => {
    return [...document.querySelectorAll('[role="option"], [role="menuitem"], [role="listbox"] *, [class*="option" i]')]
      .map((e) => (e.textContent || '').trim()).filter(Boolean).slice(0, 30);
  });
  console.log(`  dropdown #${i + 1} options:`, JSON.stringify(popped));
  // close by pressing Escape
  await page.keyboard.press('Escape');
  await new Promise((r) => setTimeout(r, 300));
}

await page.screenshot({ path: '/tmp/walk8-wizard-full.png', fullPage: true });

// 6) Look for radio groups / toggle chips specifically around the word "format"
const formatArea = await page.evaluate(() => {
  const nodes = [...document.querySelectorAll('*')]
    .filter((e) => /format/i.test(e.textContent || '') && e.children.length < 8 && (e.textContent || '').length < 400);
  return nodes.slice(0, 20).map((e) => ({
    tag: e.tagName,
    cls: e.className.toString().slice(0, 80),
    text: (e.textContent || '').trim().slice(0, 200),
  }));
});
console.log('\n---- elements mentioning "format" ----');
formatArea.forEach((f) => console.log(' ·', JSON.stringify(f)));

// 7) Dump all network calls to see what the wizard is actually using
console.log('\n---- walt.id / credentials.walt.id calls observed ----');
for (const c of calls.slice(0, 50)) {
  console.log(` ${c.status || '---'}  ${c.m}  ${c.u}`);
  if (c.body) console.log('       body:', c.body.slice(0, 300));
  if (c.resp && c.resp.length < 400) console.log('       resp:', c.resp.slice(0, 300));
}

await browser.close();
