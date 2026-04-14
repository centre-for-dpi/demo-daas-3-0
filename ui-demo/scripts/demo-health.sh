#!/usr/bin/env bash
# Health check for all demo services. Prints UP/DOWN for each.
set +e

check() {
  local name="$1"
  local url="$2"
  local code
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "$url" 2>/dev/null)
  if [[ "$code" =~ ^[234] ]]; then
    printf "  %-40s UP   (HTTP %s)\n" "$name" "$code"
  else
    printf "  %-40s DOWN (HTTP %s)\n" "$name" "${code:-timeout}"
  fi
}

echo "=========================================="
echo "  Demo Health Check"
echo "=========================================="

echo ""
echo "Walt.id stack:"
check "Wallet API"          "http://localhost:7001/"
check "Issuer API"          "http://localhost:7002/"
check "Verifier API"        "http://localhost:7003/"
check "Postgres (walletdb)" "http://localhost:5432/" # not HTTP but will 000

echo ""
echo "Inji stack:"
check "Inji Certify"            "http://localhost:8090/v1/certify/.well-known/openid-credential-issuer"
check "Certify-Nginx (HTTP)"    "http://localhost:8091/.well-known/did.json"
check "Inji Verify Service"     "http://localhost:8082/v1/verify/actuator/health"
check "Inji Verify UI"          "http://localhost:3001/"

echo ""
echo "Identity providers:"
check "Keycloak"            "http://localhost:8180/"
check "WSO2 IS"             "https://localhost:9443/"

echo ""
echo "Data sources:"
check "Citizens DB (postgres)" "http://localhost:5435/" # not HTTP

echo ""
echo "Translation:"
check "LibreTranslate"      "http://localhost:5000/languages"

echo ""
echo "App server:"
check "App"                 "http://localhost:8080/api/capabilities"
