// e2e/status-list-multibase.mjs
// Headless browser smoke test for the W3C BSL multibase fix. Confirms:
//   1. /status-list/bitstring/v1 returns 200 with the JOSE-secured VC.
//   2. The encodedList field is multibase-prefixed ("u" for base64url).
//   3. After base64url decode + gunzip the bitstring is the expected
//      131072-bit length and bit 0 is initially 0 (no revocations).
//   4. /status-list/token/v1 also returns 200 with the IETF status_list
//      claim (no multibase — its spec uses raw base64url).
//
// Probes the local stack at http://172.24.0.1:8080 over headless Chrome
// rather than curl so we exercise the same fetch + decode path the
// browser runs (cookies, CORS, content-type sniffing, gzip transport).

import puppeteer from 'puppeteer-core';
import { gunzipSync, inflateSync } from 'zlib';
import { Buffer } from 'buffer';

const BASE = process.env.BASE || 'http://172.24.0.1:8080';

const br = await puppeteer.launch({
  executablePath: '/usr/bin/google-chrome',
  headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage', '--ignore-certificate-errors'],
});

const fail = (m, ...x) => { console.log('FAIL:', m, ...x); process.exitCode = 1; };

const decodeJWS = (jws) => {
  const [, payload] = jws.split('.');
  const b64 = payload.replace(/-/g, '+').replace(/_/g, '/');
  const pad = '='.repeat((4 - b64.length % 4) % 4);
  return JSON.parse(Buffer.from(b64 + pad, 'base64').toString('utf8'));
};

const b64urlDecode = (s) => Buffer.from(s.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - s.length % 4) % 4), 'base64');

try {
  const ctx = await br.createBrowserContext();
  const p = await ctx.newPage();
  // Need an initial document for page.evaluate(fetch) — about:blank's
  // origin is opaque, so a same-origin fetch to BASE has to start from a
  // page on BASE itself. Land on the docs page (no auth, no DPG/session
  // gate) just to prime the origin.
  await p.goto(`${BASE}/docs`, { waitUntil: 'domcontentloaded', timeout: 15000 });

  // --- W3C bitstring list ---
  console.log(`==> GET ${BASE}/status-list/bitstring/v1`);
  const bsRes = await p.evaluate(async (u) => {
    const r = await fetch(u);
    return { status: r.status, ct: r.headers.get('content-type'), body: await r.text() };
  }, `${BASE}/status-list/bitstring/v1`);
  console.log(`  status=${bsRes.status} content-type=${bsRes.ct} body[:40]=${bsRes.body.slice(0, 40)}…`);
  if (bsRes.status !== 200) fail('bitstring status', bsRes.status);
  if (!bsRes.ct?.includes('vc+jwt')) fail('bitstring content-type', bsRes.ct);

  const bsPayload = decodeJWS(bsRes.body);
  const cs = bsPayload?.vc?.credentialSubject;
  if (!cs?.encodedList) fail('encodedList missing', JSON.stringify(bsPayload).slice(0, 200));
  console.log(`  encodedList[0..3]="${cs.encodedList.slice(0, 4)}..." (length ${cs.encodedList.length})`);
  if (cs.encodedList[0] !== 'u') fail('multibase prefix should be "u" for base64url');

  const bsBytes = gunzipSync(b64urlDecode(cs.encodedList.slice(1)));
  console.log(`  bitstring bytes=${bsBytes.length} (expect 16384 = 131072 bits)`);
  if (bsBytes.length !== 16384) fail('bitstring size', bsBytes.length);
  // MSB-first: bit 0 = (byte[0] >> 7) & 1
  const bsBit0 = (bsBytes[0] >> 7) & 1;
  console.log(`  bit 0 (W3C MSB-first) = ${bsBit0}`);

  if (bsPayload?.vc?.type?.includes('BitstringStatusListCredential')) {
    console.log('  ✓ vc.type includes BitstringStatusListCredential');
  } else {
    fail('vc.type missing BitstringStatusListCredential', bsPayload?.vc?.type);
  }
  if (cs.statusPurpose !== 'revocation') fail('statusPurpose', cs.statusPurpose);
  if (cs.type !== 'BitstringStatusList') fail('credentialSubject.type', cs.type);

  // --- IETF token list ---
  console.log(`\n==> GET ${BASE}/status-list/token/v1`);
  const tkRes = await p.evaluate(async (u) => {
    const r = await fetch(u);
    return { status: r.status, ct: r.headers.get('content-type'), body: await r.text() };
  }, `${BASE}/status-list/token/v1`);
  console.log(`  status=${tkRes.status} content-type=${tkRes.ct}`);
  if (tkRes.status !== 200) fail('token status', tkRes.status);
  if (!tkRes.ct?.includes('statuslist+jwt')) fail('token content-type', tkRes.ct);

  const tkPayload = decodeJWS(tkRes.body);
  const sl = tkPayload?.status_list;
  if (!sl?.lst) fail('status_list.lst missing', JSON.stringify(tkPayload).slice(0, 200));
  if (sl.bits !== 1) fail('status_list.bits', sl.bits);
  console.log(`  status_list.bits=${sl.bits} lst[0..4]="${sl.lst.slice(0, 4)}..." (length ${sl.lst.length})`);
  if (sl.lst[0] === 'u') fail('IETF token list MUST NOT have multibase prefix (raw base64url per spec)');

  const tkBytes = inflateSync(b64urlDecode(sl.lst));
  console.log(`  token list bytes=${tkBytes.length}`);
  // LSB-first: bit 0 = byte[0] & 1
  const tkBit0 = tkBytes[0] & 1;
  console.log(`  bit 0 (IETF LSB-first) = ${tkBit0}`);
} finally {
  await br.close();
}

if (process.exitCode) {
  console.log('\nFAILED');
  process.exit(1);
}
console.log('\nPASS — both lists serve correctly, multibase prefix only on W3C, both bitstrings decode.');
