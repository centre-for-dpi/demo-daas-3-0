// M11 matrix: runs every per-DPG e2e suite in sequence against a single
// registry-mode server. Acts as the final acceptance gate — if this passes,
// every milestone's real-backend path is live.
//
// Usage:
//   1. Ensure the compose stack is up (walt.id, inji-*, libretranslate, keycloak, wso2is).
//   2. Start verifiably-go in registry mode:
//        VERIFIABLY_ADDR=:8089 VERIFIABLY_ADAPTER=registry \
//        VERIFIABLY_PUBLIC_URL=http://localhost:8089 ./verifiably
//   3. VERIFIABLY_URL=http://localhost:8089 node e2e/matrix-test.mjs

import { execSync } from 'child_process';
import path from 'path';

const suites = [
  'waltid-test.mjs',
  'inji-test.mjs',
  'injiverify-test.mjs',
  'injiweb-test.mjs',
  'scan-upload-test.mjs',
  'auth-test.mjs',
  'i18n-test.mjs',
  // Chromium-driven UI checks for the two controls that auth-test.mjs and
  // i18n-test.mjs only covered at the HTTP-endpoint level (fetch, no browser).
  'chromium-lang-test.mjs',
  'chromium-auth-test.mjs',
];

const results = [];
for (const suite of suites) {
  console.log('\n' + '='.repeat(60));
  console.log('▶ ' + suite);
  console.log('='.repeat(60));
  try {
    execSync('node ' + path.join('e2e', suite), { stdio: 'inherit', env: process.env });
    results.push({ suite, ok: true });
  } catch (e) {
    results.push({ suite, ok: false });
  }
}

console.log('\n' + '='.repeat(60));
console.log('MATRIX SUMMARY');
console.log('='.repeat(60));
for (const r of results) {
  console.log(`${r.ok ? 'PASS' : 'FAIL'}  ${r.suite}`);
}
const failed = results.filter((r) => !r.ok).length;
if (failed > 0) {
  console.log(`\n${failed}/${results.length} suites failed`);
  process.exit(1);
} else {
  console.log(`\nAll ${results.length} suites passed ✓`);
}
