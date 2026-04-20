// Probe our local /issuer/schema with auth, dump schemas + one card's fields.
import puppeteer from 'puppeteer-core';
const CHROME = '/usr/bin/google-chrome';
const BASE = 'http://localhost:8080';
const KC_USER = 'admin', KC_PASS = 'admin';
const br = await puppeteer.launch({ executablePath: CHROME, headless: 'new', args: ['--no-sandbox'] });
const page = await br.newPage();
await page.setViewport({ width: 1500, height: 1200 });

try {
  await page.goto(BASE + '/', { waitUntil: 'networkidle2' });
  await page.click('button.role-card[value="issuer"]');
  await new Promise(r => setTimeout(r, 800));
  await page.waitForFunction(() => /\/auth/.test(location.pathname), { timeout: 10000 });
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(() => null),
    page.evaluate(() => {
      const btn = [...document.querySelectorAll('button.provider-btn')].find(
        (b) => (b.getAttribute('hx-vals') || '').includes('"keycloak"'));
      btn?.click();
    }),
  ]);
  await page.waitForSelector('input[name="username"]', { timeout: 15000 });
  await page.type('input[name="username"]', KC_USER);
  await page.type('input[name="password"]', KC_PASS);
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 20000 }).catch(() => null),
    page.click('input[type="submit"], button[type="submit"]'),
  ]);
  await page.waitForNetworkIdle({ idleTime: 500, timeout: 5000 }).catch(()=>{});

  // Pick walt DPG
  if (!/issuer\/schema/.test(page.url())) {
    await page.goto(BASE + '/issuer/dpg', { waitUntil: 'networkidle2' });
  }
  await new Promise(r => setTimeout(r, 500));
  if (await page.$('.dpg-card[data-vendor="Walt Community Stack"]')) {
    await page.click('.dpg-card[data-vendor="Walt Community Stack"]');
    await new Promise(r => setTimeout(r, 500));
    await page.click('#issuer-dpg-continue').catch(() => {});
    await new Promise(r => setTimeout(r, 1000));
  }
  if (!/issuer\/schema/.test(page.url())) {
    await page.goto(BASE + '/issuer/schema', { waitUntil: 'networkidle2' });
  }
  await new Promise(r => setTimeout(r, 1500));

  // Dump cards
  const cards = await page.evaluate(() =>
    [...document.querySelectorAll('.schema-card')].map(c => ({
      id: c.dataset.id, name: c.dataset.name, std: c.dataset.std,
      desc: (c.querySelector('.desc')?.textContent || '').trim().slice(0, 120),
    })));
  console.log(`TOTAL CARDS: ${cards.length}`);
  cards.slice(0, 12).forEach(c => console.log(' ·', c.id, '   name=', c.name, '  std=', c.std, '  desc=', c.desc));

  // Pick IdentityCredential card to see fields
  const idCard = cards.find(c => /identity/i.test(c.name)) || cards[0];
  if (idCard) {
    console.log('\nselecting card:', idCard.id);
    await page.evaluate((id) => {
      const c = [...document.querySelectorAll('.schema-card')].find(x => x.dataset.id === id);
      c?.querySelector('button.btn.small:not(.ghost)')?.click() || c?.querySelector('button.btn.small')?.click();
    }, idCard.id);
    await new Promise(r => setTimeout(r, 800));
    await page.goto(BASE + '/issuer/mode', { waitUntil: 'networkidle2' });
    await new Promise(r => setTimeout(r, 500));
    // Select single + wallet
    await page.evaluate(() => document.querySelector('button[hx-vals*="single"]')?.click());
    await new Promise(r => setTimeout(r, 400));
    await page.evaluate(() => document.querySelector('button[hx-vals*="wallet"]')?.click());
    await new Promise(r => setTimeout(r, 400));
    await page.goto(BASE + '/issuer/issue', { waitUntil: 'networkidle2' });
    await new Promise(r => setTimeout(r, 1000));

    const formInfo = await page.evaluate(() => ({
      title: document.querySelector('.page-title')?.textContent?.trim(),
      sub: document.querySelector('.subtitle')?.textContent?.trim(),
      fields: [...document.querySelectorAll('#single-form-el .field')].map(f => ({
        label: f.querySelector('label')?.textContent?.trim(),
        type: f.querySelector('input')?.type,
        name: f.querySelector('input')?.name,
      })),
    }));
    console.log('\n---- /issuer/issue form ----');
    console.log('title:', formInfo.title);
    console.log('sub:', formInfo.sub);
    console.log('fields:');
    formInfo.fields.forEach(f => console.log(' ·', f.name, ':', f.type, '  label:', f.label));
  }
} catch (e) {
  console.error('FATAL:', e.message);
} finally {
  await br.close();
}
