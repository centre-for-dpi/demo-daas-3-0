// Headless regression: switch language to French, navigate to inner pages,
// assert we see French text (not English).

import puppeteer from 'puppeteer-core';

const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';
const BASE = process.env.VERIFIABLY_URL || 'http://localhost:8080';

function log(ok, msg, detail) {
  console.log((ok ? 'PASS' : 'FAIL') + '  ' + msg + (detail ? ' — ' + detail : ''));
  if (!ok) process.exitCode = 1;
}
const settle = (p) => p.waitForNetworkIdle({ idleTime: 500, timeout: 6000 }).catch(() => {});

const browser = await puppeteer.launch({
  executablePath: CHROME, headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
const page = await browser.newPage();

try {
  await page.goto(BASE + '/', { waitUntil: 'networkidle2' });
  // Switch language to French via the topbar form
  await page.evaluate(() => {
    const f = document.getElementById('lang-form');
    if (!f) return;
    const sel = f.querySelector('[name="lang"]');
    sel.value = 'fr';
    f.submit();
  });
  await settle(page);
  const homeText = await page.evaluate(() => document.body.innerText);
  log(/Démarrez|Délivrance|Émetteur/.test(homeText),
      'home page translated to French', homeText.slice(0, 120).replace(/\n/g, ' '));

  // Go to issuer flow: click the Issuer role-card
  const issuerBtn = await page.$('button.role-card[value="issuer"]');
  await issuerBtn.click();
  await settle(page);
  const authText = await page.evaluate(() => document.body.innerText);
  log(/connecter|Authentifi|émetteur|fournisseur/i.test(authText),
      '/auth page translated to French', authText.slice(0, 160).replace(/\n/g, ' '));

  // Walk through role → auth on the holder side and check that the DPG
  // selection page renders in French. We don't actually authenticate (that
  // requires OIDC round-trip) — we just verify text on each unauth'd step is
  // translated.
  await page.goto(BASE + '/', { waitUntil: 'networkidle2' });
  const holderBtn = await page.$('button.role-card[value="holder"]');
  await holderBtn.click();
  await settle(page);
  const holderAuthText = await page.evaluate(() => document.body.innerText);
  log(/Connexion|détenteur|tulaire|fournisseur/i.test(holderAuthText),
      'holder /auth page translated', holderAuthText.slice(0, 160).replace(/\n/g, ' '));

  // Verifier side
  await page.goto(BASE + '/', { waitUntil: 'networkidle2' });
  const verifBtn = await page.$('button.role-card[value="verifier"]');
  await verifBtn.click();
  await settle(page);
  const verifAuthText = await page.evaluate(() => document.body.innerText);
  log(/Connexion|Vérifi|fournisseur/i.test(verifAuthText),
      'verifier /auth page translated', verifAuthText.slice(0, 160).replace(/\n/g, ' '));
} catch (e) {
  console.error('FATAL:', e.message);
  process.exit(2);
} finally {
  await browser.close();
}
