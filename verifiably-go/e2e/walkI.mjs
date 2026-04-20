// Take a screenshot of /verifier/verify to diagnose the layout issue.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const page = await br.newPage();
await page.setViewport({ width: 1400, height: 1200 });

async function auth(page) {
  await page.goto('http://localhost:8080/', { waitUntil: 'networkidle2' });
  await page.click('button.role-card[value="verifier"]');
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

try {
  await auth(page);
  if (!/verifier\/verify/.test(page.url())) {
    await page.goto('http://localhost:8080/verifier/dpg', { waitUntil: 'networkidle2' });
    if (await page.$('.dpg-card[data-vendor="Walt Community Stack"]')) {
      await page.click('.dpg-card[data-vendor="Walt Community Stack"]');
      await new Promise(r => setTimeout(r, 300));
      await page.click('#verifier-dpg-continue').catch(()=>{});
      await new Promise(r => setTimeout(r, 800));
    }
  }
  await page.goto('http://localhost:8080/verifier/verify', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 800));

  const info = await page.evaluate(() => {
    const list = document.querySelector('#verifier-custom-body .schema-list');
    if (!list) return { error: 'no schema-list' };
    const cs = window.getComputedStyle(list);
    const firstCard = list.querySelector('.schema-card');
    const cardCS = firstCard && window.getComputedStyle(firstCard);
    return {
      schema_list_display: cs.display,
      schema_list_grid_template_columns: cs.gridTemplateColumns,
      schema_list_width: list.getBoundingClientRect().width,
      form_width: document.getElementById('custom-template-form').getBoundingClientRect().width,
      card_count: list.querySelectorAll('.schema-card').length,
      first_card_width: firstCard?.getBoundingClientRect().width,
      first_card_display: cardCS?.display,
      viewport_width: document.documentElement.clientWidth,
    };
  });
  console.log(JSON.stringify(info, null, 2));
  await page.screenshot({ path: '/tmp/walkI-verifier.png', fullPage: true });
  console.log('screenshot → /tmp/walkI-verifier.png');
} catch (e) {
  console.error('FATAL:', e.message);
} finally {
  await br.close();
}
