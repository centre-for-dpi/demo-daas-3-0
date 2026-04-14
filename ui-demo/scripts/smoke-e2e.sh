#!/usr/bin/env bash
# Full E2E smoke test for whichever demo mode is currently running.
# Exercises: login → onboard issuer → issue credential → claim to wallet →
# verify. Supports both Walt.id mode (OID4VP session) and Inji mode
# (direct-verify).
set -e

APP_URL=${APP_URL:-http://localhost:8080}
JAR=/tmp/smoke-cookie.jar
rm -f "$JAR"

echo "=========================================="
echo "  Demo E2E Smoke Test"
echo "  App: $APP_URL"
echo "=========================================="

echo
echo "--- Capabilities ---"
CAPS=$(curl -s "$APP_URL/api/capabilities")
ISSUER=$(echo "$CAPS" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("issuerName",""))')
WALLET=$(echo "$CAPS" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("walletName",""))')
VERIFIER=$(echo "$CAPS" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("verifierName",""))')
echo "  issuer:   $ISSUER"
echo "  wallet:   $WALLET"
echo "  verifier: $VERIFIER"

echo
echo "--- 1. Sign up ---"
EMAIL="smoke-$(date +%s)@test.local"
STATUS=$(curl -s -c "$JAR" -b "$JAR" -X POST "$APP_URL/login" \
  -d "login_type=real&auth_action=signup&role=issuer&name=Smoke&email=$EMAIL&password=Test1234!" \
  -o /dev/null -w '%{http_code}')
echo "  login HTTP $STATUS  (303 = success redirect)"

echo
echo "--- 2. Onboard issuer ---"
DID=$(curl -s -b "$JAR" -X POST "$APP_URL/api/issuer/onboard?keyType=secp256r1" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin).get("issuerDid",""))')
echo "  issuer DID: $DID"

echo
echo "--- 3. Pick a credential config ---"
# Use FarmerCredentialV2 if Inji, else UniversityDegree_jwt_vc_json if walt.
CONFIGS=$(curl -s -b "$JAR" "$APP_URL/api/credential-types")
if echo "$CONFIGS" | grep -q FarmerCredentialV2; then
  CONFIG_ID=FarmerCredentialV2
  FORMAT=ldp_vc
  CLAIMS='{"fullName":"Jelagat Gitau","farmerID":"KE-FID-001","dateOfBirth":"1988-05-12"}'
else
  CONFIG_ID=UniversityDegree_jwt_vc_json
  FORMAT=jwt_vc_json
  CLAIMS='{"name":"Jelagat Gitau","degree":"BSc","major":"Computer Science"}'
fi
echo "  config: $CONFIG_ID ($FORMAT)"

echo
echo "--- 4. Issue credential offer ---"
ISSUE=$(curl -s -b "$JAR" -X POST "$APP_URL/api/credential/issue" \
  -H 'Content-Type: application/json' \
  -d "{\"configId\":\"$CONFIG_ID\",\"format\":\"$FORMAT\",\"claims\":$CLAIMS}")
OFFER=$(echo "$ISSUE" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("offerUrl",""))')
echo "  offer: ${OFFER:0:90}..."

echo
echo "--- 5. Claim into wallet ---"
CLAIM=$(curl -s -b "$JAR" -X POST "$APP_URL/api/wallet/claim-offer" \
  -H 'Content-Type: application/json' \
  -d "{\"offerUrl\":\"$OFFER\"}")
echo "  $CLAIM"

echo
echo "--- 6. List wallet credentials ---"
WALLET_OUT=$(curl -s -b "$JAR" "$APP_URL/api/wallet/credentials")
COUNT=$(echo "$WALLET_OUT" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')
CRED_ID=$(echo "$WALLET_OUT" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d[0]["id"] if d else "")')
echo "  credentials: $COUNT | id: $CRED_ID"

echo
echo "--- 7. Verify ---"
# Inji verifier exposes direct-verify; walt.id requires OID4VP session.
if [[ "$VERIFIER" == *"Inji"* ]]; then
  echo "  (using direct-verify path)"
  RESULT=$(curl -s -b "$JAR" -X POST "$APP_URL/api/verifier/direct-verify" \
    -H 'Content-Type: application/json' \
    -d "{\"credentialId\":\"$CRED_ID\"}")
  VERIFIED=$(echo "$RESULT" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("verified",False))')
  echo "  verified: $VERIFIED"
else
  echo "  (using OID4VP session path)"
  VRESP=$(curl -s -b "$JAR" -X POST "$APP_URL/api/verifier/verify" \
    -H 'Content-Type: application/json' \
    -d '{"credentialTypes":["VerifiableCredential"]}')
  STATE=$(echo "$VRESP" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("state",""))')
  REQ_URL=$(echo "$VRESP" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("request_url",""))')
  curl -s -b "$JAR" -X POST "$APP_URL/api/wallet/present" \
    -H 'Content-Type: application/json' \
    -d "{\"presentationRequest\":\"$REQ_URL\"}" -o /dev/null
  RESULT=$(curl -s -b "$JAR" "$APP_URL/api/verifier/session/$STATE")
  VERIFIED=$(echo "$RESULT" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("verified",False))')
  echo "  verified: $VERIFIED"
fi

echo
if [ "$VERIFIED" = "True" ]; then
  echo "=========================================="
  echo "  ✓ E2E PASSED"
  echo "=========================================="
else
  echo "=========================================="
  echo "  ✗ E2E FAILED"
  echo "=========================================="
  exit 1
fi
