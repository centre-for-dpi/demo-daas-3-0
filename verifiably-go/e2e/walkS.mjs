// Verify: wallet grid renders 3 wide, type filter narrows, delete button removes a card.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const p = await br.newPage();
await p.setViewport({ width: 1500, height: 1400 });

async function auth() {
  await p.goto('http://localhost:8080/', { waitUntil: 'networkidle2' });
  await p.click('button.role-card[value="holder"]');
  await p.waitForFunction(() => /\/auth/.test(location.pathname), { timeout: 10000 });
  await Promise.all([
    p.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(()=>null),
    p.evaluate(() => [...document.querySelectorAll('button.provider-btn')].find(b => (b.getAttribute('hx-vals')||'').includes('keycloak'))?.click()),
  ]);
  await p.waitForSelector('input[name="username"]', { timeout: 15000 });
  await p.type('input[name="username"]', 'admin');
  await p.type('input[name="password"]', 'admin');
  await Promise.all([
    p.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(()=>null),
    p.click('input[type="submit"], button[type="submit"]'),
  ]);
  await new Promise(r => setTimeout(r, 700));
}

try {
  await auth();
  await p.goto('http://localhost:8080/holder/dpg', { waitUntil: 'networkidle2' });
  const card = await p.$('.dpg-card[data-vendor="Walt Community Stack"]');
  if (card) { await card.click(); await new Promise(r => setTimeout(r, 300)); await p.click('#holder-dpg-continue').catch(()=>{}); await new Promise(r => setTimeout(r, 900)); }
  await p.goto('http://localhost:8080/holder/wallet', { waitUntil: 'networkidle2' });
  await p.waitForSelector('[data-wallet-card]', { timeout: 15000 });
  await new Promise(r => setTimeout(r, 400));

  const layout = await p.evaluate(() => {
    const grid = document.querySelector('.wallet-grid');
    const cs = grid ? window.getComputedStyle(grid) : null;
    const cards = grid ? grid.querySelectorAll('[data-wallet-card]').length : 0;
    return {
      grid_display: cs?.display,
      grid_template_columns: cs?.gridTemplateColumns,
      cards_total: cards,
      first_card_has_delete: !!grid?.querySelector('[data-wallet-card] button[hx-post*="/holder/wallet/delete"]'),
      type_filter_options: [...document.querySelectorAll('[data-wallet-type-filter] option')].map(o => o.value),
    };
  });
  console.log('layout:', JSON.stringify(layout, null, 2));

  // Type filter narrows
  await p.select('[data-wallet-type-filter]', 'Bank Id');
  await new Promise(r => setTimeout(r, 200));
  const afterFilter = await p.evaluate(() => {
    const cards = [...document.querySelectorAll('[data-wallet-card]')];
    return {
      total: cards.length,
      visible: cards.filter(c => c.style.display !== 'none').length,
      counter: document.getElementById('wallet-search-count')?.textContent?.trim(),
    };
  });
  console.log('after "Bank Id" filter:', afterFilter);

  // Reset + delete the first visible card
  await p.select('[data-wallet-type-filter]', 'all');
  await new Promise(r => setTimeout(r, 150));
  // Dump the button's attributes to verify HTMX wiring
  const btnAttrs = await p.evaluate(() => {
    const btn = document.querySelector('[data-wallet-card] button[hx-post*="/holder/wallet/delete"]');
    if (!btn) return null;
    const attrs = {};
    for (const a of btn.attributes) attrs[a.name] = a.value;
    return {
      attrs,
      textContent: btn.textContent.trim(),
      htmx_loaded: !!window.htmx,
    };
  });
  console.log('delete button attrs:', JSON.stringify(btnAttrs, null, 2));

  const beforeDelete = await p.evaluate(() => document.querySelectorAll('[data-wallet-card]').length);
  // Direct fetch bypassing hx-confirm.
  const delResult = await p.evaluate(async () => {
    const btn = document.querySelector('[data-wallet-card] button[hx-post*="/holder/wallet/delete"]');
    const vals = JSON.parse(btn.getAttribute('hx-vals'));
    const body = new URLSearchParams(vals).toString();
    const resp = await fetch('/holder/wallet/delete', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body,
    });
    return { status: resp.status, body: (await resp.text()).slice(0, 200) };
  });
  console.log('delete response:', delResult);
  // Reload to see the post-delete wallet state
  await p.reload({ waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 400));
  await p.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 1200));
  const afterDelete = await p.evaluate(() => document.querySelectorAll('[data-wallet-card]').length);
  console.log(`delete: before=${beforeDelete} after=${afterDelete} (reduced=${beforeDelete - afterDelete})`);

  await p.screenshot({ path: '/tmp/walkS-wallet-grid.png', fullPage: true });
  console.log('screenshot → /tmp/walkS-wallet-grid.png');
} finally { await br.close(); }
