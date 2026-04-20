// Verify the wallet search: type a query, confirm cards hide correctly.
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

  async function probe(q) {
    await p.evaluate((v) => {
      const s = document.querySelector('[data-wallet-search]');
      s.value = v;
      s.dispatchEvent(new Event('input', {bubbles:true}));
    }, q);
    await new Promise(r => setTimeout(r, 150));
    return p.evaluate(() => {
      const cards = [...document.querySelectorAll('[data-wallet-card]')];
      const visible = cards.filter(c => c.style.display !== 'none');
      return {
        total: cards.length,
        visible: visible.length,
        titles: visible.map(c => c.querySelector('h4')?.textContent?.trim()).slice(0, 5),
        empty_shown: document.getElementById('wallet-search-empty')?.style.display === 'block',
        counter: document.getElementById('wallet-search-count')?.textContent?.trim(),
      };
    });
  }

  console.log('empty query:', await probe(''));
  console.log('"identity":', await probe('identity'));
  console.log('"bank":', await probe('bank'));
  console.log('"sd_jwt":', await probe('sd_jwt'));
  console.log('"nonexistent-xyz":', await probe('nonexistent-xyz'));
  console.log('"jojo":', await probe('jojo'));  // claim value search
} finally { await br.close(); }
