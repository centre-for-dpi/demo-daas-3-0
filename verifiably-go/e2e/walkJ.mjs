// Drive the verifier flow through the UI and confirm the generated PD
// carries the full vct URL (not a bare type name).
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const page = await br.newPage();
await page.setViewport({ width: 1500, height: 1200 });

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
      await new Promise(r => setTimeout(r, 400));
      await page.click('#verifier-dpg-continue').catch(()=>{});
      await new Promise(r => setTimeout(r, 900));
    }
  }
  await page.goto('http://localhost:8080/verifier/verify', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 800));

  // Pick Bank Id, vc+sd-jwt
  await page.evaluate(() => {
    const card = [...document.querySelectorAll('#verifier-custom-body .schema-card')]
      .find(c => c.dataset.name === 'Bank Id');
    const chip = [...card.querySelectorAll('.chip.small')].find(x => x.title === 'vc+sd-jwt');
    chip?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 400, timeout: 5000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 500));

  // Generate
  await page.evaluate(() => {
    const btn = [...document.querySelectorAll('#custom-template-form button[type="submit"]')]
      .find(b => /Generate/i.test(b.textContent));
    btn?.click();
  });
  await page.waitForNetworkIdle({ idleTime: 500, timeout: 10000 }).catch(()=>{});
  await new Promise(r => setTimeout(r, 1000));

  const authURI = await page.evaluate(() => document.querySelector('#oid4vp-output .link-display')?.textContent?.trim());
  console.log('full auth URI:', authURI);

  // Extract PD URI
  const pdURI = new URL(authURI).searchParams.get('presentation_definition_uri');
  console.log('\nPD URL:', pdURI);
  const pdResp = await fetch(pdURI);
  const pd = await pdResp.json();
  console.log('\nPD:', JSON.stringify(pd, null, 2));

  const pattern = pd.input_descriptors?.[0]?.constraints?.fields?.[0]?.filter?.pattern;
  console.log('\nvct pattern:', pattern);
  if (pattern && pattern.startsWith('http')) {
    console.log('✓ full URL vct — wallet match should now succeed');
  } else {
    console.log('✗ still using short type name — match will fail');
  }
} catch (e) {
  console.error('FATAL:', e.message);
} finally {
  await br.close();
}
