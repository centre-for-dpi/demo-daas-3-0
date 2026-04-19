// End-to-end test of verifiably-go via headless Chromium.
// Walks through all three roles (issuer, holder, verifier) and exercises
// the HTMX-driven interactions the user would perform.
//
// Usage: VERIFIABLY_URL=http://localhost:8080 node e2e/browser-test.mjs

import puppeteer from 'puppeteer-core';

const BASE = process.env.VERIFIABLY_URL || 'http://localhost:8080';
const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';

const results = [];
const fail = [];

function log(ok, msg, detail) {
  const prefix = ok ? 'PASS' : 'FAIL';
  const line = detail ? `${prefix}  ${msg} — ${detail}` : `${prefix}  ${msg}`;
  console.log(line);
  results.push({ ok, msg, detail });
  if (!ok) fail.push({ msg, detail });
}

async function expect(cond, msg, detail) {
  log(!!cond, msg, cond ? '' : detail);
}

// Wait for HTMX to settle on a page that just swapped.
async function settle(page, extraMs = 50) {
  await page.waitForNetworkIdle({ idleTime: 120, timeout: 3000 }).catch(() => {});
  if (extraMs) await new Promise((r) => setTimeout(r, extraMs));
}

async function click(page, selector) {
  await page.waitForSelector(selector, { visible: true, timeout: 5000 });
  // Ensure the element is in the viewport before clicking. Puppeteer's auto-
  // scroll sometimes leaves elements partially above the viewport, which makes
  // page.click's computed coords land outside the element.
  await page.$eval(selector, (el) =>
    el.scrollIntoView({ block: 'center', behavior: 'instant' })
  );
  await page.click(selector);
}

async function textOf(page, selector) {
  return page.$eval(selector, (el) => el.textContent.trim()).catch(() => null);
}

async function run() {
  const browser = await puppeteer.launch({
    executablePath: CHROME,
    headless: 'new',
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  });
  const page = await browser.newPage();
  if (process.env.VERBOSE) {
    page.on('request', (r) => console.log(`  > ${r.method()} ${r.url()}`));
    page.on('response', (r) => console.log(`  < ${r.status()} ${r.url()}`));
  }
  page.on('pageerror', (e) => log(false, 'uncaught JS error', e.message));
  page.on('console', (msg) => {
    if (msg.type() !== 'error') return;
    const text = msg.text();
    // Ignore favicon 404s — we don't ship one.
    if (/favicon\.ico/.test(text)) return;
    if (/Failed to load resource.*404/.test(text)) return;
    log(false, 'console.error', text);
  });

  try {
    // ====== Landing ======
    await page.goto(BASE, { waitUntil: 'networkidle0' });
    const title = await page.title();
    await expect(/Verifiably/.test(title), 'landing title contains Verifiably', title);

    const roleCardCount = await page.$$eval('.role-card', (els) => els.length);
    await expect(roleCardCount === 3, 'landing shows 3 role cards', `got ${roleCardCount}`);

    // ====== ISSUER ======
    await page.goto(BASE, { waitUntil: 'networkidle0' });
    await click(page, 'button.role-card[value="issuer"]');
    await page.waitForNavigation({ waitUntil: 'networkidle0' }).catch(() => {});
    await expect(/\/auth$/.test(page.url()), 'issuer role → /auth', page.url());

    // Complete auth
    await click(page, 'form[action="/auth"] button[type="submit"]');
    await page.waitForNavigation({ waitUntil: 'networkidle0' });
    await expect(/\/issuer\/dpg$/.test(page.url()), 'auth → /issuer/dpg', page.url());

    // Expand a DPG card, then Continue
    const dpgCount = await page.$$eval('#issuer-dpg-grid .dpg-card', (els) => els.length);
    await expect(dpgCount >= 2, 'issuer DPG grid shows ≥2 cards', `got ${dpgCount}`);

    // Initial render should have exactly one issuer-dpg-continue
    const initialContinueCount = await page.$$eval('#issuer-dpg-continue', (els) => els.length);
    await expect(initialContinueCount === 1,
      'issuer DPG page has exactly one #issuer-dpg-continue on load',
      `got ${initialContinueCount}`);

    // Click first DPG to expand (HTMX swaps innerHTML)
    await page.click('#issuer-dpg-grid .dpg-card');
    await settle(page, 200);
    const expanded = await page.$('.dpg-card.expanded');
    await expect(!!expanded, 'clicking a DPG card expands it');

    // After HTMX toggle response, still exactly one continue button
    const afterToggleCount = await page.$$eval('#issuer-dpg-continue', (els) => els.length);
    await expect(afterToggleCount === 1,
      'still one #issuer-dpg-continue after toggle (no duplicate)',
      `got ${afterToggleCount}`);

    // Continue button should no longer be disabled
    const disabledAfter = await page.$eval('#issuer-dpg-continue', (el) =>
      el.classList.contains('btn-disabled')
    );
    await expect(!disabledAfter, 'continue button enables after DPG selection');

    await page.click('#issuer-dpg-continue');
    await page.waitForNavigation({ waitUntil: 'networkidle0' });
    await expect(/\/issuer\/schema$/.test(page.url()), 'commit DPG → /issuer/schema');

    // Schema browser: select first schema
    const schemaCardCount = await page.$$eval('.schema-card', (els) => els.length);
    await expect(schemaCardCount > 0, 'schema list populated', `got ${schemaCardCount}`);

    // "Show JSON" on first card → JSON preview appears
    const firstCardSelector = '.schema-card:first-child';
    await click(page, `${firstCardSelector} .btn-row button:first-child`);
    await settle(page, 200);
    const jsonPreview = await page.$('.json-preview');
    await expect(!!jsonPreview, 'Show JSON renders inline JSON preview');
    const previewText = jsonPreview ? await jsonPreview.evaluate((el) => el.textContent) : '';
    await expect(
      /"\$schema".*"properties"/s.test(previewText),
      'JSON preview contains $schema + properties keys'
    );

    // Search filter
    await page.type('input[name="q"]', 'degree');
    await settle(page, 500);
    const filteredCount = await page.$$eval('.schema-card', (els) => els.length);
    await expect(filteredCount >= 1 && filteredCount < schemaCardCount,
      'search "degree" narrows schema list', `went ${schemaCardCount} → ${filteredCount}`);

    // Clear search
    await page.$eval('input[name="q"]', (el) => { el.value = ''; el.dispatchEvent(new Event('input', { bubbles: true })); });
    await settle(page, 400);

    // Pick "Use this schema" on first card
    await click(page, '.schema-card:first-child button[hx-post="/issuer/schema/select"]');
    await settle(page, 300);
    const selectedCard = await page.$('.schema-card.selected');
    await expect(!!selectedCard, 'selecting a schema marks it .selected');

    // Continue to mode
    const continueButtons = await page.$$('a#continue-btn');
    await expect(continueButtons.length === 1,
      'only one #continue-btn in DOM (no duplicate id)',
      `found ${continueButtons.length}`);
    // Use waitUntil:'load' here — networkidle0 sometimes hangs on this
    // specific nav because fonts/CDN connections stay momentarily open.
    await Promise.all([
      page.waitForNavigation({ waitUntil: 'load' }),
      click(page, 'a#continue-btn'),
    ]);
    await expect(/\/issuer\/mode$/.test(page.url()), 'continue → /issuer/mode');

    // Selection feedback: clicking "Bulk from CSV" should highlight that card
    // and un-highlight "Single subject". We assert via computed border-color
    // since styling is driven by :has(input[type="radio"]:checked).
    const accent = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--accent').trim()
    );
    const borderOf = (name, val) => page.evaluate(([n, v]) => {
      const card = [...document.querySelectorAll('.option-card')]
        .find((c) => c.querySelector(`input[name="${n}"][value="${v}"]`));
      return card ? getComputedStyle(card).borderColor : null;
    }, [name, val]);
    const hex2rgb = (h) => {
      const s = h.replace('#', '');
      return `rgb(${parseInt(s.slice(0,2),16)}, ${parseInt(s.slice(2,4),16)}, ${parseInt(s.slice(4,6),16)})`;
    };
    const accentRGB = accent.startsWith('#') ? hex2rgb(accent) : accent;

    // Baseline: single is selected (initial state)
    const singleBorderBefore = await borderOf('scale', 'single');
    const bulkBorderBefore = await borderOf('scale', 'bulk');
    await expect(singleBorderBefore === accentRGB,
      'single card has accent border on load', `${singleBorderBefore} vs ${accentRGB}`);
    await expect(bulkBorderBefore !== accentRGB,
      'bulk card does NOT have accent border on load', bulkBorderBefore);

    // Click bulk — bulk should become accent, single should lose accent.
    // Wait past the 0.25s border-color transition before sampling.
    await page.evaluate(() => {
      const card = [...document.querySelectorAll('.option-card')]
        .find((c) => c.querySelector('input[name="scale"][value="bulk"]'));
      card.click();
    });
    await new Promise((r) => setTimeout(r, 400));
    const singleBorderAfter = await borderOf('scale', 'single');
    const bulkBorderAfter = await borderOf('scale', 'bulk');
    await expect(bulkBorderAfter === accentRGB,
      'after click: bulk card has accent border', bulkBorderAfter);
    await expect(singleBorderAfter !== accentRGB,
      'after click: single card loses accent border (only one selected)',
      singleBorderAfter);

    // Reset to single for the remaining flow
    await page.evaluate(() => {
      const card = [...document.querySelectorAll('.option-card')]
        .find((c) => c.querySelector('input[name="scale"][value="single"]'));
      card.click();
    });
    await new Promise((r) => setTimeout(r, 100));

    // Submit mode form with defaults (single + wallet)
    await click(page, 'button[type="submit"].btn');
    await page.waitForNavigation({ waitUntil: 'networkidle0' });
    await expect(/\/issuer\/issue$/.test(page.url()), 'mode → /issuer/issue');

    // The form should have inputs named field_<name>
    const fieldInputs = await page.$$eval('input[name^="field_"]', (els) => els.length);
    await expect(fieldInputs > 0, 'issue form has prefilled fields', `got ${fieldInputs}`);

    // Click "Issue credential" — HTMX posts and renders #issue-result
    await click(page, 'button[hx-post="/issuer/issue"]');
    await settle(page, 400);
    const offerURI = await textOf(page, '#issue-result .link-display');
    await expect(
      offerURI && offerURI.startsWith('openid-credential-offer://'),
      'issuance produces openid-credential-offer URI',
      offerURI && offerURI.slice(0, 60)
    );

    // ====== HOLDER ======
    await page.goto(BASE, { waitUntil: 'networkidle0' });
    await click(page, 'button.role-card[value="holder"]');
    await page.waitForNavigation({ waitUntil: 'networkidle0' });
    await click(page, 'form[action="/auth"] button[type="submit"]');
    await page.waitForNavigation({ waitUntil: 'networkidle0' });
    await expect(/\/holder\/dpg$/.test(page.url()), 'holder flow → /holder/dpg');

    // Expand the walt.id wallet card (non-redirect)
    const holderCards = await page.$$eval('.dpg-card', (els) =>
      els.map((el) => ({ text: el.textContent.trim().slice(0, 80) }))
    );
    await expect(holderCards.length >= 2, 'holder DPG grid populated', `got ${holderCards.length}`);
    // Find walt.id Web Wallet (non-redirect)
    const waltIdx = await page.$$eval('.dpg-card', (els) =>
      els.findIndex((el) => /walt\.id Web Wallet/.test(el.textContent))
    );
    await expect(waltIdx >= 0, 'walt.id Web Wallet present');
    const cards = await page.$$('.dpg-card');
    await cards[waltIdx].click();
    await settle(page, 200);

    await click(page, '#holder-dpg-continue, button[hx-post="/holder/dpg"]');
    await page.waitForNavigation({ waitUntil: 'networkidle0' }).catch(() => {});
    await expect(/\/holder\/wallet$/.test(page.url()), 'holder DPG → /holder/wallet', page.url());

    // Empty paste — the server should respond with an error toast but NOT
    // wipe the wallet. Previously this returned an empty 200 body which HTMX
    // swapped into #wallet-body, making the page appear to vanish.
    const walletHtmlBefore = await page.$eval('#wallet-body', (el) => el.innerHTML.length);
    await page.$eval('#offer-paste', (el) => { el.value = ''; });
    const emptyPasteResponse = page.waitForResponse(
      (r) => r.url().endsWith('/holder/wallet/paste') && r.request().method() === 'POST',
      { timeout: 5000 }
    );
    await click(page, 'form[hx-post="/holder/wallet/paste"] button[type="submit"]');
    const emptyResp = await emptyPasteResponse;
    await settle(page, 200);
    await expect(emptyResp.headers()['hx-reswap'] === 'none',
      'empty paste sends HX-Reswap: none',
      `got ${emptyResp.headers()['hx-reswap']}`);
    const walletHtmlAfter = await page.$eval('#wallet-body', (el) => el.innerHTML.length);
    await expect(walletHtmlAfter >= walletHtmlBefore * 0.9,
      'empty paste does NOT wipe #wallet-body',
      `before=${walletHtmlBefore} after=${walletHtmlAfter}`);

    // Wallet: click "Paste example" to prefill, then "Process offer"
    await click(page, 'button[hx-post="/holder/wallet/example"]');
    await settle(page, 300);
    const pastedValue = await page.$eval('#offer-paste', (el) => el.value);
    await expect(
      pastedValue.startsWith('openid-credential-offer://'),
      'paste-example fills textarea'
    );

    await click(page, 'button[type="submit"][class*="btn"]');
    await settle(page, 500);
    const pendingCount = await page.$$eval('.wallet-cred.pending', (els) => els.length);
    await expect(pendingCount >= 1, 'process offer creates a pending credential');

    // Accept the first pending offer. Wait explicitly for the network request
    // to complete (settle's idleTime of 120ms can return before HTMX fires its
    // POST).
    const acceptResponse = page.waitForResponse(
      (r) => r.url().endsWith('/holder/wallet/accept') && r.request().method() === 'POST',
      { timeout: 5000 }
    );
    await click(page, '.wallet-cred.pending button[hx-post="/holder/wallet/accept"]');
    await acceptResponse;
    await settle(page, 300);
    const pendingAfterAccept = await page.$$eval('.wallet-cred.pending', (els) => els.length);
    const heldAfterAccept = await page.$$eval('.wallet-cred.accepted', (els) => els.length);
    await expect(pendingAfterAccept === 0, 'pending cleared after accept');
    await expect(heldAfterAccept >= 2, 'held list grew after accept', `held=${heldAfterAccept}`);

    // Simulate scan (adds new pending), reject it
    const scanResponse = page.waitForResponse(
      (r) => r.url().endsWith('/holder/wallet/scan') && r.request().method() === 'POST',
      { timeout: 5000 }
    );
    await click(page, 'button[hx-post="/holder/wallet/scan"]');
    await scanResponse;
    await settle(page, 300);
    const pendingFromScan = await page.$$eval('.wallet-cred.pending', (els) => els.length);
    await expect(pendingFromScan >= 1, 'simulate scan adds pending');
    const rejectResponse = page.waitForResponse(
      (r) => r.url().endsWith('/holder/wallet/reject') && r.request().method() === 'POST',
      { timeout: 5000 }
    );
    await click(page, '.wallet-cred.pending button[hx-post="/holder/wallet/reject"]');
    await rejectResponse;
    await settle(page, 300);
    const pendingAfterReject = await page.$$eval('.wallet-cred.pending', (els) => els.length);
    await expect(pendingAfterReject === 0, 'reject clears pending');

    // ====== VERIFIER ======
    await page.goto(BASE, { waitUntil: 'networkidle0' });
    await click(page, 'button.role-card[value="verifier"]');
    await page.waitForNavigation({ waitUntil: 'networkidle0' });
    await click(page, 'form[action="/auth"] button[type="submit"]');
    await page.waitForNavigation({ waitUntil: 'networkidle0' });
    await expect(/\/verifier\/dpg$/.test(page.url()), 'verifier flow → /verifier/dpg');

    // Pick walt.id Verifier (non-redirect)
    const verifierIdx = await page.$$eval('.dpg-card', (els) =>
      els.findIndex((el) => /walt\.id Verifier/.test(el.textContent))
    );
    const vCards = await page.$$('.dpg-card');
    await vCards[verifierIdx].click();
    await settle(page, 200);
    await click(page, '#verifier-dpg-continue, button[hx-post="/verifier/dpg"]');
    await page.waitForNavigation({ waitUntil: 'networkidle0' }).catch(() => {});
    await expect(/\/verifier\/verify$/.test(page.url()), 'verifier DPG → /verifier/verify');

    // Generate OID4VP request (default template)
    await click(page, 'button[hx-post="/verifier/verify/request"]');
    await settle(page, 400);
    const requestURI = await textOf(page, '#oid4vp-output .link-display');
    await expect(
      requestURI && requestURI.startsWith('openid4vp://'),
      'OID4VP request generator produces openid4vp:// URI',
      requestURI && requestURI.slice(0, 60)
    );

    // Simulate holder response
    await click(page, '#oid4vp-output button[hx-post="/verifier/verify/response"]');
    await settle(page, 400);
    const banner = await page.$('#verify-result .verify-banner');
    await expect(!!banner, 'verify-result renders banner');
    const bannerValidOrInvalid = await page.$eval('#verify-result .verify-banner', (el) =>
      el.classList.contains('valid') || el.classList.contains('invalid')
    );
    await expect(bannerValidOrInvalid, 'banner shows valid or invalid state');

    // Direct verify — scan (M6+: camera-driven; can't run getUserMedia in
    // headless without a fake stream, so we exercise the handler directly
    // with a synthetic credential_data that matches what jsQR would produce).
    await page.evaluate(async () => {
      const form = new FormData();
      form.append('method', 'scan');
      form.append('credential_data', 'synthetic-scan-payload');
      const resp = await fetch('/verifier/verify/direct', {
        method: 'POST', body: form,
        headers: { 'HX-Request': 'true' },
      });
      const html = await resp.text();
      document.getElementById('verify-result').innerHTML = html;
    });
    await settle(page, 400);
    const scanBanner = await page.$('#verify-result .verify-banner');
    await expect(!!scanBanner, 'scan direct-verify renders banner');

    // Direct verify — paste needs text
    await page.type('textarea[name="credential_data"]', 'eyJhbGciOiJFZERTQSJ9.xxx.yyy');
    await click(page, 'form[hx-post="/verifier/verify/direct"] button[type="submit"]');
    await settle(page, 400);
    const pasteResult = await page.$('#verify-result .verify-banner');
    await expect(!!pasteResult, 'paste direct-verify renders banner');

    // ====== THEME TOGGLE ======
    await page.goto(BASE, { waitUntil: 'networkidle0' });
    const initialTheme = await page.$eval('html', (el) => el.getAttribute('data-theme'));
    await click(page, '.theme-toggle');
    await new Promise((r) => setTimeout(r, 100));
    const toggledTheme = await page.$eval('html', (el) => el.getAttribute('data-theme'));
    await expect(
      initialTheme !== toggledTheme,
      'theme toggle flips data-theme attribute',
      `${initialTheme} → ${toggledTheme}`
    );

    // ====== SCHEMA BUILDER ======
    // Re-enter issuer flow to test the custom schema builder
    await page.goto(`${BASE}/issuer/schema/build`, { waitUntil: 'networkidle0' });
    const builderForm = await page.$('#single-form-wrap, form');
    await expect(!!builderForm, 'schema builder page loads');

    // Add a field
    const addFieldBtn = await page.$('button[hx-post="/issuer/schema/build/add-field"]');
    if (addFieldBtn) {
      const fieldsBefore = await page.$$eval('input[name^="field_name_"]', (e) => e.length);
      await addFieldBtn.click();
      await settle(page, 300);
      const fieldsAfter = await page.$$eval('input[name^="field_name_"]', (e) => e.length);
      await expect(fieldsAfter === fieldsBefore + 1, 'add-field increases field count',
        `${fieldsBefore} → ${fieldsAfter}`);
    }

    console.log('\n' + '='.repeat(60));
    console.log(`Results: ${results.filter((r) => r.ok).length}/${results.length} passed`);
    if (fail.length) {
      console.log('\nFailures:');
      fail.forEach((f) => console.log(`  - ${f.msg}${f.detail ? ' — ' + f.detail : ''}`));
      process.exitCode = 1;
    }
  } catch (err) {
    console.error('\nFATAL:', err.message);
    console.error(err.stack);
    process.exitCode = 2;
  } finally {
    await browser.close();
  }
}

run();
