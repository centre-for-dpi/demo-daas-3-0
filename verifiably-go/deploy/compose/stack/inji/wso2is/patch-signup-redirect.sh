#!/bin/sh
# patch-signup-redirect.sh — runs INSIDE the wso2is container, in the
# background, just after WSO2 starts. Injects a meta-refresh redirect
# into the self-registration "Account created successfully" page so
# users who finish the WSO2 signup UX get taken back to verifiably-go's
# /auth screen instead of landing on a static success page with no
# escape.
#
# Why this exists, and why it's an in-container patch rather than a
# config-file setting:
#
#   WSO2 IS 7's accountrecoveryendpoint webapp renders the post-signup
#   success page from a JSP that lives inside accountrecoveryendpoint.war.
#   The page is intentionally OIDC-flow-detached (self-registration
#   isn't part of the OIDC spec), so there's no `redirect_uri` parameter
#   threaded through and no deployment.toml setting that controls
#   "where to send the user after signup". Older WSO2 docs hint at
#   `sign_up_callback_url` keys, but they're not honored in IS 7.0.
#
#   The reliable fix is to patch the JSP itself. Since the WAR is
#   extracted at runtime to a path under
#   /home/wso2carbon/.../webapps/accountrecoveryendpoint/, this script
#   polls for the extracted directory, finds whichever JSP contains the
#   "Account created successfully" string, and injects a
#   <meta http-equiv="refresh"> tag near the top of <body>.
#
#   Idempotent: a marker comment in the patched JSP lets a re-run skip
#   already-patched files. Restart-safe: the WAR re-extracts on every
#   container restart (clobbering our patch), and this script runs on
#   every restart, so the patch comes back automatically.
#
# Driven by the SIGNUP_REDIRECT_URL env var the compose file sets to
# http://${VERIFIABLY_PUBLIC_HOST}:${VERIFIABLY_HOST_PORT}/auth.

set -e

WEBAPP_DIR=/home/wso2carbon/wso2is-7.0.0/repository/deployment/server/webapps/accountrecoveryendpoint
REDIRECT_URL="${SIGNUP_REDIRECT_URL:-http://localhost:8080/auth}"
DELAY_SECONDS="${SIGNUP_REDIRECT_DELAY:-3}"
MARKER="data-verifiably-signup-redirect"

# Wait up to 10 minutes for WSO2 to extract the WAR. On a slow VPS, the
# webapp deployment can take 5+ minutes after the JVM starts. The exit
# code is 0 either way — we don't want this background script to take
# down the wso2is container if the timing's off.
TARGET=""
for i in $(seq 1 120); do
  if [ -d "$WEBAPP_DIR" ]; then
    # Find a JSP whose rendered output includes the success message.
    # WSO2 names this JSP differently across IS versions, so we grep
    # for the string the user actually sees rather than hardcoding a
    # filename.
    TARGET=$(grep -lr "Account created successfully" "$WEBAPP_DIR" 2>/dev/null | grep -E '\.jsp$' | head -1 || true)
    [ -n "$TARGET" ] && break
  fi
  sleep 5
done

if [ -z "$TARGET" ]; then
  echo "patch-signup-redirect: no success JSP found in $WEBAPP_DIR after 10 min — skipping" >&2
  exit 0
fi

if grep -q "$MARKER" "$TARGET"; then
  echo "patch-signup-redirect: $TARGET already patched — no-op"
  exit 0
fi

# Inject just after the first <body opener. Use | as the sed delimiter so
# the URL's slashes don't need escaping. Restrict to the FIRST <body
# match via the 0,/RE/ address form so we don't double-inject if a JSP
# accidentally has multiple body tags.
sed -i \
  "0,/<body/{s|<body|<body $MARKER=\"true\"><meta http-equiv=\"refresh\" content=\"${DELAY_SECONDS};url=${REDIRECT_URL}\"><!-- redirect injected by verifiably-go: returns the user to the OIDC client after WSO2 self-registration completes -->|}" \
  "$TARGET"

echo "patch-signup-redirect: injected meta-refresh → $REDIRECT_URL into $TARGET"
