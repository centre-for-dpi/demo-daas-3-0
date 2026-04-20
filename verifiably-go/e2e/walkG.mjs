// Verify the new card-based verifier custom-request flow.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const page = await br.newPage();
await page.setViewport({ width: 1500, height: 1400 });

try {
  // Auth as verifier
  await page.goto('http://localhost:8080/', { waitUntil: 'networkidle2' });
  await page.click('button.role-card[value="verifier"]');
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
  if (!/verifier\/verify/.test(page.url())) {
    await page.goto('http://localhost:8080/verifier/dpg', { waitUntil: 'networkidle2' });
    if (await page.$('.dpg-card[data-vendor="Walt Community Stack"]')) {
      await page.click('.dpg-card[data-vendor="Walt Community Stack"]');
      await new Promise(r => setTimeout(r, 400));
      await page.click('#verifier-dpg-continue').catch(() => {});
      await new Promise(r => setTimeout(r, 900));
    }
  }
  if (!/verifier\/verify/.test(page.url())) {
    await page.goto('http://localhost:8080/verifier/verify', { waitUntil: 'networkidle2' });
  }
  await new Promise(r => setTimeout(r, 1000));

  const state = await page.evaluate(() => {
    const body = document.getElementById('verifier-custom-body');
    const stdChips = [...body?.querySelectorAll('.chip-row .chip.small') || []].slice(0, 6).map(c => (c.textContent||'').trim());
    const searchBox = body?.querySelector('input[type="search"]');
    const cards = [...body?.querySelectorAll('.schema-card') || []].map(c => ({
      name: c.dataset.name,
      selected: c.classList.contains('selected'),
      chips: [...c.querySelectorAll('.chip.small')].map(x => x.title || x.textContent.trim()),
    }));
    return {
      body_present: !!body,
      std_chips_top: stdChips,
      search_present: !!searchBox,
      total_cards: cards.length,
      first_3: cards.slice(0, 3),
      has_identity: !!cards.find(c => c.name === 'Identity Credential'),
    };
  });
  console.log('initial state:', JSON.stringify(state, null, 2));

  // Click a format chip on the "Identity Credential" card
  await page.evaluate(() => {
    const cards = [...document.querySelectorAll('#verifier-custom-body .schema-card')];
    const id = cards.find(c => c.dataset.name === 'Identity Credential');
    const chip = [...id.querySelectorAll('.chip.small')].find(x => x.title === 'vc+sd-jwt');
    chip?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 800));

  const afterPick = await page.evaluate(() => {
    const card = [...document.querySelectorAll('#verifier-custom-body .schema-card')]
      .find(c => c.dataset.name === 'Identity Credential');
    const preview = document.getElementById('custom-template-preview');
    const hidden = document.querySelector('input[name="schema_id"]');
    return {
      card_selected: card?.classList.contains('selected'),
      active_chip: card ? [...card.querySelectorAll('.chip.active')].map(c => c.title) : [],
      hidden_schema_id: hidden?.value,
      preview_has_fields: !!preview?.querySelector('input[type="checkbox"]'),
      preview_field_count: preview ? preview.querySelectorAll('input[type=checkbox]').length : 0,
      preview_text_snip: (preview?.innerText || '').slice(0, 250),
    };
  });
  console.log('\nafter picking vc+sd-jwt on Identity Credential:');
  console.log(JSON.stringify(afterPick, null, 2));

  // Now use the std filter "sd_jwt_vc (IETF)" — target the chip-row that's
  // DIRECTLY above the search input (not inside a card).
  await page.evaluate(() => {
    const searchInput = document.querySelector('#verifier-custom-body input[type="search"]');
    const filterWrapper = searchInput?.parentElement;
    const chip = [...filterWrapper?.querySelectorAll('.chip-row .chip') || []]
      .find(c => /sd_jwt_vc/i.test(c.textContent || ''));
    chip?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 600));
  const afterFilter = await page.evaluate(() => {
    const cards = [...document.querySelectorAll('#verifier-custom-body .schema-card')];
    return { count: cards.length, first_5: cards.slice(0, 5).map(c => c.dataset.name) };
  });
  console.log('\nafter std filter sd_jwt_vc (IETF):', JSON.stringify(afterFilter));

  // Clear to all
  await page.evaluate(() => {
    const searchInput = document.querySelector('#verifier-custom-body input[type="search"]');
    const chip = [...searchInput?.parentElement?.querySelectorAll('.chip-row .chip') || []]
      .find(c => (c.textContent || '').trim() === 'all');
    chip?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 500));

  await page.type('#verifier-custom-body input[type="search"]', 'pass');
  await page.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 800));
  const afterSearch = await page.evaluate(() => {
    const cards = [...document.querySelectorAll('#verifier-custom-body .schema-card')];
    return { count: cards.length, names: cards.map(c => c.dataset.name) };
  });
  console.log('\nafter search \"pass\":', JSON.stringify(afterSearch));

  // Clear search, re-pick Identity Credential, click Generate and verify an
  // openid4vp URI comes back.
  await page.evaluate(() => {
    const s = document.querySelector('#verifier-custom-body input[type="search"]');
    s.value = '';
    s.dispatchEvent(new Event('input', {bubbles: true}));
  });
  await page.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await page.evaluate(() => {
    const card = [...document.querySelectorAll('#verifier-custom-body .schema-card')]
      .find(c => c.dataset.name === 'Identity Credential');
    const chip = [...card.querySelectorAll('.chip.small')].find(x => x.title === 'vc+sd-jwt');
    chip?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 400));

  // Click Generate
  await page.evaluate(() => {
    const btn = [...document.querySelectorAll('#custom-template-form button[type="submit"]')]
      .find(b => /Generate/i.test(b.textContent));
    btn?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 600, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 1000));

  const output = await page.evaluate(() => {
    const out = document.getElementById('oid4vp-output');
    const link = out?.querySelector('.link-display');
    return {
      has_qr: !!out?.querySelector('img[alt*="QR"]'),
      link: (link?.textContent || '').slice(0, 120),
    };
  });
  console.log('\ngenerated presentation request:', JSON.stringify(output));
} catch (e) {
  console.error('FATAL:', e.message);
} finally {
  await br.close();
}
