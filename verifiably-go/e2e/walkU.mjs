// Walk every canned example in walt.id's verifier swagger /openid4vc/verify
// endpoint and dump their bodies; grep for ldp_vc / formats used.
import puppeteer from 'puppeteer-core';
const br = await puppeteer.launch({ executablePath: '/usr/bin/google-chrome', headless: 'new', args: ['--no-sandbox'] });
const p = await br.newPage();
await p.setViewport({ width: 1500, height: 1400 });
try {
  await p.goto('http://localhost:7003/swagger/index.html#/Credential%20Verification/post_openid4vc_verify', { waitUntil: 'networkidle2' });
  await new Promise(r => setTimeout(r, 3000));
  // Expand operation
  await p.evaluate(() => document.querySelector('.opblock-summary')?.click());
  await new Promise(r => setTimeout(r, 800));

  // Read all example bodies
  const examples = await p.evaluate(() => {
    const sel = document.querySelector('.examples-select-element');
    if (!sel) return [];
    const out = [];
    for (const o of sel.options) {
      sel.value = o.value;
      sel.dispatchEvent(new Event('change', {bubbles: true}));
      // grab the visible example body from the nearest code block
      const pre = document.querySelector('.example, pre.body-param__example, pre');
      out.push({ name: o.textContent, body: pre ? pre.textContent.slice(0, 600) : '' });
    }
    return out;
  });

  // Print a concise summary: example name → formats it mentions
  for (const e of examples) {
    const formats = [];
    for (const fmt of ['jwt_vc_json-ld','jwt_vc_json','vc+sd-jwt','dc+sd-jwt','mso_mdoc','ldp_vc','ldp_vp','jwt_vc','jwt_vp']) {
      if (e.body.includes(`"${fmt}"`)) formats.push(fmt);
    }
    if (formats.length) console.log(`  • "${e.name}" — ${formats.join(', ')}`);
    else console.log(`  • "${e.name}" — (no format literals found in body snippet)`);
  }
} finally { await br.close(); }
