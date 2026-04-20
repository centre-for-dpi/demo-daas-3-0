// Reload /issuer/schema 3 times and compare the name order. Should be identical.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const page = await br.newPage();

async function grabOrder() {
  await page.goto('http://localhost:8080/issuer/schema', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 800));
  return await page.evaluate(() =>
    [...document.querySelectorAll('.schema-card')].map(c => c.dataset.name));
}

try {
  // Auth
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
  await new Promise(r => setTimeout(r, 800));
  if (!/issuer\/schema/.test(page.url())) {
    await page.goto('http://localhost:8080/issuer/dpg', { waitUntil: 'networkidle2' });
    if (await page.$('.dpg-card[data-vendor="Walt Community Stack"]')) {
      await page.click('.dpg-card[data-vendor="Walt Community Stack"]');
      await new Promise(r => setTimeout(r, 400));
      await page.click('#issuer-dpg-continue').catch(() => {});
      await new Promise(r => setTimeout(r, 900));
    }
  }

  const a = await grabOrder();
  const b = await grabOrder();
  const c = await grabOrder();
  console.log('reload 1 (first 10):', a.slice(0, 10));
  console.log('reload 2 (first 10):', b.slice(0, 10));
  console.log('reload 3 (first 10):', c.slice(0, 10));
  const sameAB = JSON.stringify(a) === JSON.stringify(b);
  const sameAC = JSON.stringify(a) === JSON.stringify(c);
  console.log('\norder stable across reloads?', sameAB && sameAC);
  console.log('total cards:', a.length);

  // Also verify clicking a format chip doesn't shuffle
  await page.goto('http://localhost:8080/issuer/schema', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 800));
  const before = await page.evaluate(() =>
    [...document.querySelectorAll('.schema-card')].map(c => c.dataset.name));
  // Click the "Use this schema" button on the 5th card
  await page.evaluate(() => {
    const card = document.querySelectorAll('.schema-card')[4];
    const btn = [...card.querySelectorAll('button.btn.small')].find(b => /use this/i.test(b.textContent));
    btn?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 600));
  const after = await page.evaluate(() =>
    [...document.querySelectorAll('.schema-card')].map(c => c.dataset.name));
  console.log('\nbefore click (5th):', before[4]);
  console.log('after  click (5th):', after[4]);
  console.log('order preserved through click?', JSON.stringify(before) === JSON.stringify(after));
} catch (e) {
  console.error('FATAL:', e.message);
} finally {
  await br.close();
}
