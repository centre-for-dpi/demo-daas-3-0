// Verify: issuer schema card shows all 6+ formats with issue-only warnings;
// verifier card shows only the 4-5 presentation-capable formats.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });

async function auth(page, role) {
  await page.goto('http://localhost:8080/', { waitUntil: 'networkidle2' });
  await page.click(`button.role-card[value="${role}"]`);
  await page.waitForFunction(() => /\/auth/.test(location.pathname), { timeout: 10000 });
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(() => null),
    page.evaluate(() => [...document.querySelectorAll('button.provider-btn')].find(b => (b.getAttribute('hx-vals')||'').includes('keycloak'))?.click()),
  ]);
  await page.waitForSelector('input[name="username"]', { timeout: 15000 });
  await page.type('input[name="username"]', 'admin');
  await page.type('input[name="password"]', 'admin');
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(() => null),
    page.click('input[type="submit"], button[type="submit"]'),
  ]);
  await new Promise(r => setTimeout(r, 700));
}

async function probe(role, path, cardName) {
  const page = await (await br.createBrowserContext()).newPage();
  await page.setViewport({ width: 1500, height: 1200 });
  await auth(page, role);
  await page.goto(`http://localhost:8080/${role}/dpg`, { waitUntil: 'networkidle2' });
  if (await page.$('.dpg-card[data-vendor="Walt Community Stack"]')) {
    await page.click('.dpg-card[data-vendor="Walt Community Stack"]');
    await new Promise(r => setTimeout(r, 300));
    await page.click(`#${role}-dpg-continue`).catch(() => {});
    await new Promise(r => setTimeout(r, 900));
  }
  if (!new RegExp(`/${role}/`).test(page.url())) await page.goto(`http://localhost:8080${path}`, { waitUntil: 'networkidle2' });
  else if (!page.url().includes(path)) await page.goto(`http://localhost:8080${path}`, { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 600));
  const chips = await page.evaluate(({name, role}) => {
    const container = role === 'issuer'
      ? document.querySelector('#schema-list')
      : document.querySelector('#verifier-custom-body .schema-list');
    const cards = [...(container?.querySelectorAll('.schema-card') || [])];
    const card = cards.find(c => (c.dataset.name || c.querySelector('h4')?.textContent.trim()) === name);
    if (!card) return { err: 'no card' };
    return {
      count: card.querySelectorAll('.chip').length,
      chips: [...card.querySelectorAll('.chip')].map(c => ({
        text: c.textContent.trim(),
        title: c.title,
      })),
    };
  }, {name: cardName, role});
  await page.close();
  return chips;
}

try {
  console.log('== ISSUER / Bank Id ==');
  console.log(JSON.stringify(await probe('issuer', '/issuer/schema', 'Bank Id'), null, 2));
  console.log('\n== VERIFIER / Bank Id ==');
  console.log(JSON.stringify(await probe('verifier', '/verifier/verify', 'Bank Id'), null, 2));
} finally {
  await br.close();
}
