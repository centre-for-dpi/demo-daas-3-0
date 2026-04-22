// Extract the exact <select> options shown in walt.id's verifier swagger UI
// for the /openid4vc/verify request_credentials format field.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const p = await br.newPage();
await p.setViewport({ width: 1400, height: 1200 });
try {
  await p.goto('http://localhost:7003/swagger/index.html#/Credential%20Verification/post_openid4vc_verify', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 3000));

  // Expand the operation by clicking its header
  await p.evaluate(() => {
    const header = document.querySelector('.opblock-summary');
    header?.click();
  });
  await new Promise(r => setTimeout(r, 800));

  // Try to click "Schema" / "Model" / "Try it out" tabs so <select>s appear
  await p.evaluate(() => {
    [...document.querySelectorAll('button, .tab')].forEach(b => {
      const t = (b.textContent || '').toLowerCase();
      if (t.includes('try it out') || t.includes('schema')) b.click();
    });
  });
  await new Promise(r => setTimeout(r, 800));

  // Grab every <select> on the page with its options
  const selects = await p.evaluate(() => {
    return [...document.querySelectorAll('select')].map((s, i) => ({
      idx: i,
      name: s.name || s.getAttribute('aria-label') || '',
      options: [...s.options].map(o => o.textContent || o.value),
      outerHTMLStart: s.outerHTML.slice(0, 200),
    }));
  });
  console.log('selects on swagger page:', JSON.stringify(selects, null, 2));

  // Also look at the request body / schema section for any format enum
  const formatMentions = await p.evaluate(() => {
    const out = [];
    for (const el of document.querySelectorAll('.model, .prop-type, .property')) {
      const txt = (el.textContent || '').trim();
      if (/format/i.test(txt) && txt.length < 400) {
        out.push(txt.slice(0, 300));
      }
    }
    return out.slice(0, 10);
  });
  console.log('\nformat-adjacent text:');
  formatMentions.forEach(t => console.log('  ·', t));

  await p.screenshot({ path: '/tmp/walkT-swagger.png', fullPage: true });
} finally { await br.close(); }
