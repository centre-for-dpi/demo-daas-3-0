// Verify the format chip-row renders and a user can switch format via chip click.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const page = await br.newPage();
await page.setViewport({ width: 1500, height: 1200 });

try {
  // Auth + DPG + land on schema
  await page.goto('http://localhost:8080/', { waitUntil: 'networkidle2' });
  await page.click('button.role-card[value="issuer"]');
  await page.waitForFunction(() => /\/auth/.test(location.pathname), { timeout: 10000 });
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(() => null),
    page.evaluate(() => [...document.querySelectorAll('button.provider-btn')].find(b => (b.getAttribute('hx-vals') || '').includes('keycloak'))?.click()),
  ]);
  await page.waitForSelector('input[name="username"]', { timeout: 15000 });
  await page.type('input[name="username"]', 'admin');
  await page.type('input[name="password"]', 'admin');
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(() => null),
    page.click('input[type="submit"], button[type="submit"]'),
  ]);
  await new Promise(r => setTimeout(r, 1000));
  if (!/issuer\/schema/.test(page.url())) {
    await page.goto('http://localhost:8080/issuer/dpg', { waitUntil: 'networkidle2' });
    if (await page.$('.dpg-card[data-vendor="Walt Community Stack"]')) {
      await page.click('.dpg-card[data-vendor="Walt Community Stack"]');
      await new Promise(r => setTimeout(r, 400));
      await page.click('#issuer-dpg-continue').catch(() => {});
      await new Promise(r => setTimeout(r, 900));
    }
  }
  if (!/issuer\/schema/.test(page.url())) {
    await page.goto('http://localhost:8080/issuer/schema', { waitUntil: 'networkidle2' });
  }
  await new Promise(r => setTimeout(r, 1000));

  // Find IdentityCredential card and enumerate its chips
  const info = await page.evaluate(() => {
    const cards = [...document.querySelectorAll('.schema-card')];
    const id = cards.find(c => c.dataset.name === 'Identity Credential');
    if (!id) return null;
    return {
      name: id.dataset.name,
      default_id: id.dataset.id,
      default_std: id.dataset.std,
      chips: [...id.querySelectorAll('.chip')].map(c => ({
        label: (c.textContent || '').trim(),
        title: c.title,
        active: c.classList.contains('active'),
        hxvals: c.getAttribute('hx-vals'),
      })),
    };
  });
  console.log('IdentityCredential card:', JSON.stringify(info, null, 2));

  // Click the vc+sd-jwt chip
  const clicked = await page.evaluate(() => {
    const cards = [...document.querySelectorAll('.schema-card')];
    const id = cards.find(c => c.dataset.name === 'Identity Credential');
    const chip = [...id.querySelectorAll('.chip')].find(c => c.title === 'vc+sd-jwt');
    chip?.click();
    return !!chip;
  });
  console.log('clicked vc+sd-jwt chip:', clicked);
  await page.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 800));

  // After HTMX swap, the card should now show vc+sd-jwt chip active
  const after = await page.evaluate(() => {
    const cards = [...document.querySelectorAll('.schema-card')];
    const id = cards.find(c => c.dataset.name === 'Identity Credential');
    return {
      selected: id.classList.contains('selected'),
      active_chip: [...id.querySelectorAll('.chip.active')].map(c => c.title),
      card_id_attr: id.dataset.id,
    };
  });
  console.log('after chip click:', JSON.stringify(after));

  // Continue to /issuer/mode then /issuer/issue to confirm the SD-JWT variant flows through
  await page.goto('http://localhost:8080/issuer/mode', { waitUntil: 'networkidle2' });
  await page.evaluate(() => document.querySelector('button[hx-vals*="single"]')?.click());
  await new Promise(r => setTimeout(r, 300));
  await page.evaluate(() => document.querySelector('button[hx-vals*="wallet"]')?.click());
  await new Promise(r => setTimeout(r, 300));
  await page.goto('http://localhost:8080/issuer/issue', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 600));

  const form = await page.evaluate(() => ({
    sub: document.querySelector('.subtitle')?.textContent?.trim(),
    hidden_schema: document.querySelector('input[name="schema_id"]')?.value,
    hidden_dpg: document.querySelector('input[name="issuer_dpg"]')?.value,
  }));
  console.log('issue form:', JSON.stringify(form, null, 2));
} catch (e) {
  console.error('FATAL:', e.message);
} finally {
  await br.close();
}
