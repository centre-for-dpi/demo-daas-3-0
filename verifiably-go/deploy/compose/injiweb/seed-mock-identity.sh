#!/usr/bin/env bash
# seed-mock-identity.sh — stuff a fake identity into
# mosipid/mock-identity-system so an end-to-end OIDC login is possible.
#
# The mock identity system ships with zero identities and provides an
# open POST /v1/mock-identity-system/identity endpoint for seeding. The
# user then logs in at http://localhost:3005/authorize with:
#   individualId = 8267411072
#   pin          = 111111
#
# Idempotent — re-running returns a soft error from the mock service,
# which this script swallows.

set -euo pipefail

MOCK_URL="${MOCK_IDENTITY_URL:-http://localhost:8083}"
INDIVIDUAL_ID="${MOCK_INDIVIDUAL_ID:-8267411072}"
PIN="${MOCK_PIN:-111111}"

REQ_BODY=$(INDIVIDUAL_ID="$INDIVIDUAL_ID" PIN="$PIN" python3 <<'PY'
import json, os, datetime
# mock-identity-system 0.10.1 RequestWrapper only accepts `requestTime` +
# `request` (no `id`/`version`) and requires every OIDC-standard userinfo
# field the mock authenticator might surface — missing any of them fails
# with "invalid_<field>". The list below mirrors what the mock plugin
# expects based on the validation errors it surfaces.
now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.") + f"{datetime.datetime.now(datetime.timezone.utc).microsecond // 1000:03d}Z"
individual_id = os.environ["INDIVIDUAL_ID"]
pin = os.environ["PIN"]

def ml(v):
    return [{"language": "eng", "value": v}]

body = {
    "requestTime": now,
    "request": {
        "individualId": individual_id,
        "pin": pin,
        "password": pin,
        "fullName": ml("Demo Farmer"),
        "givenName": ml("Demo"),
        "middleName": ml("K"),
        "familyName": ml("Farmer"),
        "nickName": ml("Demo"),
        "preferredUsername": ml("demofarmer"),
        "preferredLang": "eng",
        "locale": "en-US",
        "zoneInfo": "Africa/Nairobi",
        "dateOfBirth": "1984/05/05",
        "gender": ml("Male"),
        "email": "demo.farmer@example.com",
        "phone": "+254700123456",
        "streetAddress": ml("1 Nyeri Road"),
        "locality": ml("Nyeri"),
        "region": ml("Central"),
        "postalCode": "10100",
        "country": ml("KEN"),
        "encodedPhoto": "",
    },
}
print(json.dumps(body))
PY
)

echo "POST $MOCK_URL/v1/mock-identity-system/identity (individualId=$INDIVIDUAL_ID)"
RESPONSE=$(curl -sS \
    -H "Content-Type: application/json" \
    -d "$REQ_BODY" \
    "$MOCK_URL/v1/mock-identity-system/identity" || true)

echo "$RESPONSE" | python3 -m json.tool 2>/dev/null || echo "$RESPONSE"

# The response always contains an "errors" key (possibly empty). Success
# signal is a non-null "response" object with errors == [].
if echo "$RESPONSE" | grep -qE '"errors":\s*\[\s*\]' && echo "$RESPONSE" | grep -qE '"response":\s*\{'; then
    echo "OK: identity $INDIVIDUAL_ID seeded (PIN: $PIN)"
    exit 0
fi
if echo "$RESPONSE" | grep -qE 'already|duplicate|exists'; then
    echo "OK: identity $INDIVIDUAL_ID already seeded"
    exit 0
fi
echo "WARN: seeding may not have succeeded — check the response body above"
exit 0
