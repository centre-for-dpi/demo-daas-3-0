// Screenshot /issuer/schema to verify the new legend + yellow ⚠ styling.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const page = await br.newPage();
await page.setViewport({ width: 1500, height: 1400 });
try {
  await page.goto('http://localhost:8080/', { waitUntil: 'networkidle2' });
  await page.click('button.role-card[value="issuer"]');
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
  await page.goto('http://localhost:8080/issuer/dpg', { waitUntil: 'networkidle2' });
  if (await page.$('.dpg-card[data-vendor="Walt Community Stack"]')) {
    await page.click('.dpg-card[data-vendor="Walt Community Stack"]');
    await new Promise(r => setTimeout(r, 300));
    await page.click('#issuer-dpg-continue').catch(()=>{});
    await new Promise(r => setTimeout(r, 900));
  }
  if (!/issuer\/schema/.test(page.url())) await page.goto('http://localhost:8080/issuer/schema', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 800));

  // Scroll to a card with ⚠ chips
  await page.evaluate(() => {
    const card = [...document.querySelectorAll('.schema-card')].find(c => c.querySelector('.format-issue-only'));
    card?.scrollIntoView({ block: 'center' });
  });
  await new Promise(r => setTimeout(r, 300));
  await page.screenshot({ path: '/tmp/walkM-issuer-legend.png', fullPage: false });

  const legend = await page.evaluate(() => {
    const el = document.querySelector('.format-legend');
    return el ? { text: el.innerText.slice(0, 200), computed: window.getComputedStyle(el.querySelector('.chip.small.format-issue-only')).color } : null;
  });
  console.log('legend:', JSON.stringify(legend));
  const chipColors = await page.evaluate(() => {
    const chips = [...document.querySelectorAll('.schema-card .chip.format-issue-only .warn-glyph')];
    return chips.slice(0, 3).map(g => window.getComputedStyle(g).color);
  });
  console.log('warn glyph colors:', chipColors);
  console.log('screenshot → /tmp/walkM-issuer-legend.png');
} finally { await br.close(); }
