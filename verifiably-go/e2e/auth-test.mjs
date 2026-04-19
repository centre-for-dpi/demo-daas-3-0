// M9 headless test: verifies the auth provider picker renders real provider
// buttons and the /auth/start endpoint issues a valid HX-Redirect pointing at
// each provider's authorize endpoint. Full OIDC login requires seeding a
// client + user in each IDP — out of scope for this test; that's covered by
// the full matrix in M11.
//
// Assertions:
//   - Both Keycloak and WSO2IS buttons appear on /auth.
//   - /auth/start with provider=keycloak returns HX-Redirect to
//     keycloak's authorize endpoint with the right query params (client_id,
//     redirect_uri, response_type=code, code_challenge_method=S256).
//   - Same for wso2is.
//
// Usage: VERIFIABLY_URL=http://localhost:8089 node e2e/auth-test.mjs

const BASE = process.env.VERIFIABLY_URL || 'http://localhost:8089';

const results = [];
const fail = [];
function log(ok, msg, detail) {
  console.log((ok ? 'PASS' : 'FAIL') + '  ' + msg + (detail ? ' — ' + detail : ''));
  results.push({ ok, msg, detail });
  if (!ok) fail.push({ msg, detail });
}
async function expect(cond, msg, detail) { log(!!cond, msg, cond ? '' : detail); }

async function run() {
  // Land on /auth — we need a role set first.
  const jar = new Map();
  function cookieHeader() {
    return Array.from(jar.entries()).map(([k, v]) => `${k}=${v}`).join('; ');
  }
  async function fetchX(url, opts = {}) {
    const headers = Object.assign({}, opts.headers || {}, { Cookie: cookieHeader() });
    const res = await fetch(url, { ...opts, headers, redirect: 'manual' });
    const setCookie = res.headers.get('set-cookie');
    if (setCookie) {
      const m = setCookie.match(/^([^=]+)=([^;]*)/);
      if (m) jar.set(m[1], m[2]);
    }
    return res;
  }

  await fetchX(BASE + '/');
  await fetchX(BASE + '/role', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: 'role=issuer',
  });

  const authPage = await fetchX(BASE + '/auth').then((r) => r.text());
  await expect(/Keycloak/.test(authPage), 'auth page lists Keycloak', '');
  await expect(/WSO2/i.test(authPage), 'auth page lists WSO2IS', '');

  // /auth/start for each provider — expect HX-Redirect to the provider.
  for (const [id, expectHost, expectPath] of [
    ['keycloak', 'localhost:8180', '/realms/master/protocol/openid-connect/auth'],
    ['wso2is', 'localhost:9443', '/oauth2/authorize'],
  ]) {
    const res = await fetchX(BASE + '/auth/start', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'HX-Request': 'true',
      },
      body: `provider=${id}`,
    });
    const hxr = res.headers.get('hx-redirect') || res.headers.get('HX-Redirect');
    await expect(!!hxr, `${id}: HX-Redirect header present`, hxr || '(missing)');
    if (hxr) {
      const u = new URL(hxr);
      await expect(u.host === expectHost, `${id}: redirect host`, `${u.host} vs ${expectHost}`);
      await expect(u.pathname === expectPath, `${id}: authorize path`, `${u.pathname} vs ${expectPath}`);
      await expect(u.searchParams.get('response_type') === 'code', `${id}: response_type=code`, '');
      await expect(u.searchParams.get('code_challenge_method') === 'S256', `${id}: PKCE S256`, '');
      await expect(u.searchParams.get('client_id') === 'verifiably-go', `${id}: client_id`, '');
      await expect(!!u.searchParams.get('state'), `${id}: state present`, '');
    }
  }

  console.log('\n' + '='.repeat(60));
  console.log(`Results: ${results.filter((r) => r.ok).length}/${results.length} passed`);
  if (fail.length) {
    console.log('\nFailures:');
    for (const f of fail) console.log(`  - ${f.msg}${f.detail ? ' — ' + f.detail : ''}`);
    process.exit(1);
  }
}

run();
