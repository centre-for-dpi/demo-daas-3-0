// M10 headless test: flipping the top-bar language selector re-renders the
// landing subtitle translated via live LibreTranslate. Asserts:
//   - English subtitle contains the English text.
//   - POST /lang=fr swaps it to French (LibreTranslate returns a word
//     specific to French for the first sentence, so we assert a sentinel
//     ASCII-fold check rather than an exact match).
//   - Same for es.
//
// Usage: VERIFIABLY_URL=http://localhost:8089 node e2e/i18n-test.mjs

const BASE = process.env.VERIFIABLY_URL || 'http://localhost:8089';

const results = [];
const fail = [];
function log(ok, msg, detail) {
  console.log((ok ? 'PASS' : 'FAIL') + '  ' + msg + (detail ? ' — ' + detail : ''));
  results.push({ ok, msg, detail });
  if (!ok) fail.push({ msg, detail });
}
async function expect(cond, msg, detail) { log(!!cond, msg, cond ? '' : detail); }

async function fetchWith(url, jar, opts = {}) {
  const headers = Object.assign({}, opts.headers || {}, { Cookie: jarCookies(jar) });
  const res = await fetch(url, { ...opts, headers, redirect: 'manual' });
  const sc = res.headers.get('set-cookie');
  if (sc) {
    const m = sc.match(/^([^=]+)=([^;]*)/);
    if (m) jar.set(m[1], m[2]);
  }
  return res;
}
function jarCookies(jar) {
  return Array.from(jar.entries()).map(([k, v]) => `${k}=${v}`).join('; ');
}

async function run() {
  // Start with a fresh jar → default language English.
  const jar = new Map();
  const en = await fetchWith(BASE + '/', jar).then((r) => r.text());
  await expect(
    /A thin, backend-agnostic interface/.test(en),
    'English: landing subtitle contains English text',
    '',
  );

  // Switch to French.
  await fetchWith(BASE + '/lang', jar, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: 'lang=fr',
  });
  const fr = await fetchWith(BASE + '/', jar).then((r) => r.text());
  await expect(
    /interface mince|Choisissez votre rôle|Choisissez votre r\\u00f4le/.test(fr),
    'French: landing subtitle is translated',
    (fr.match(/subtitle">[^<]+/) || [''])[0].slice(0, 120),
  );

  // Switch to Spanish.
  await fetchWith(BASE + '/lang', jar, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: 'lang=es',
  });
  const es = await fetchWith(BASE + '/', jar).then((r) => r.text());
  await expect(
    /Elija su|su rol|seleccione|verificable/i.test(es),
    'Spanish: landing subtitle is translated',
    (es.match(/subtitle">[^<]+/) || [''])[0].slice(0, 120),
  );

  // Back to English.
  await fetchWith(BASE + '/lang', jar, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: 'lang=en',
  });
  const back = await fetchWith(BASE + '/', jar).then((r) => r.text());
  await expect(
    /A thin, backend-agnostic/.test(back),
    'English: switching back restores English',
    '',
  );

  console.log('\n' + '='.repeat(60));
  console.log(`Results: ${results.filter((r) => r.ok).length}/${results.length} passed`);
  if (fail.length) {
    console.log('\nFailures:');
    for (const f of fail) console.log(`  - ${f.msg}${f.detail ? ' — ' + f.detail : ''}`);
    process.exit(1);
  }
}

run();
