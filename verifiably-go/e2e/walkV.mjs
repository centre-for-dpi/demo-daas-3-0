// Verify the wallet card renders disclosed claim values (not empty) for
// an SD-JWT credential. Loads the wallet, looks for a vc+sd-jwt card,
// checks it has fields rendered.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const p = await br.newPage();
await p.setViewport({ width: 1500, height: 1200 });

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

  // Filter to vc+sd-jwt and dump the first card's field rows
  await p.select('[data-wallet-format-filter]', 'vc+sd-jwt');
  await new Promise(r => setTimeout(r, 200));
  const sample = await p.evaluate(() => {
    const visible = [...document.querySelectorAll('[data-wallet-card]')].filter(c => c.style.display !== 'none');
    return visible.slice(0, 3).map(c => ({
      title: c.querySelector('h4')?.textContent?.trim(),
      format: c.getAttribute('data-format'),
      fields: [...c.querySelectorAll('dl.wallet-card-fields dt')].map((dt) => ({
        claim: dt.textContent.trim(),
        value: dt.nextElementSibling?.textContent.trim().slice(0, 40),
      })),
    }));
  });
  console.log('vc+sd-jwt cards (first 3):', JSON.stringify(sample, null, 2));
} finally { await br.close(); }
