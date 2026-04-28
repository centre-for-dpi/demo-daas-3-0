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
#   polls for the extracted directory and patches whichever JSP renders
#   the success page.
#
#   Three strategies are tried in order:
#     1. Direct string match — works only if the JSP has the message
#        text inlined (rare; most WSO2 JSPs use i18n keys).
#     2. i18n lookup — find the resource key for "Account created
#        successfully" in the .properties bundle, then find the JSP
#        that references that key via <fmt:message key="..."/>.
#     3. Filename heuristic — patch any JSP whose name suggests
#        post-registration completion (processregistration.jsp,
#        complete.jsp, signup-complete.jsp, etc.).
#
#   Idempotent: a marker comment in the patched JSP lets a re-run skip
#   already-patched files. Restart-safe: the WAR re-extracts on every
#   container restart (clobbering our patch), and this script runs on
#   every restart, so the patch comes back automatically.
#
#   Diagnostic-friendly: logs every step to /tmp/patch-signup-redirect.log
#   including which JSP candidates were considered and which one was
#   patched.

set -u  # don't set -e — we want to attempt every fallback

WEBAPP_DIR=/home/wso2carbon/wso2is-7.0.0/repository/deployment/server/webapps/accountrecoveryendpoint
REDIRECT_URL="${SIGNUP_REDIRECT_URL:-http://localhost:8080/auth}"
DELAY_SECONDS="${SIGNUP_REDIRECT_DELAY:-3}"
MARKER="data-verifiably-signup-redirect"

log() { echo "[patch-signup-redirect] $*"; }

# Wait up to 10 minutes for WSO2 to extract the WAR. On a slow VPS, the
# webapp deployment can take 5+ minutes after the JVM starts.
log "starting; waiting for $WEBAPP_DIR to be populated"
for i in $(seq 1 120); do
  if [ -d "$WEBAPP_DIR" ] && [ -n "$(find "$WEBAPP_DIR" -name '*.jsp' -print -quit 2>/dev/null)" ]; then
    log "webapp dir populated after $((i * 5))s"
    break
  fi
  sleep 5
done

if [ ! -d "$WEBAPP_DIR" ]; then
  log "ERROR: $WEBAPP_DIR never appeared — skipping"
  exit 0
fi

log "JSP files in webapp:"
find "$WEBAPP_DIR" -maxdepth 2 -name '*.jsp' -printf '  %p\n' 2>/dev/null | head -20

# ---- strategy 1: direct string match -------------------------------------
log "strategy 1: searching JSPs for the literal success message"
TARGET=$(grep -lr "Account created successfully" "$WEBAPP_DIR" 2>/dev/null | grep -E '\.jsp$' | head -1 || true)
if [ -n "$TARGET" ]; then
  log "  found via direct string match: $TARGET"
fi

# ---- strategy 2: i18n key lookup -----------------------------------------
if [ -z "$TARGET" ]; then
  log "strategy 2: searching i18n properties for the success message"
  PROP_FILE=$(grep -lr "Account created successfully" "$WEBAPP_DIR" 2>/dev/null \
              | grep -E '\.(properties|xml)$' | head -1 || true)
  if [ -n "$PROP_FILE" ]; then
    log "  message text found in: $PROP_FILE"
    # Extract the resource key. Lines look like: some.key=Account created successfully
    KEY=$(grep -E '^[^=]+=[^=]*Account created successfully' "$PROP_FILE" \
          | head -1 | cut -d= -f1 | sed 's/^ *//; s/ *$//' || true)
    if [ -n "$KEY" ]; then
      log "  resource key: $KEY"
      # Find any JSP referencing that key — tolerant of single/double quotes,
      # whitespace, fmt:message vs s:text vs i18n variants.
      ESCAPED_KEY=$(printf '%s' "$KEY" | sed 's|\.|\\.|g')
      TARGET=$(grep -lr "key=[\"']$ESCAPED_KEY[\"']\|<%[^%]*$ESCAPED_KEY[^%]*%>" \
               "$WEBAPP_DIR" 2>/dev/null | grep -E '\.jsp$' | head -1 || true)
      if [ -n "$TARGET" ]; then
        log "  found JSP via i18n key: $TARGET"
      fi
    fi
  fi
fi

# ---- strategy 3: filename heuristic --------------------------------------
if [ -z "$TARGET" ]; then
  log "strategy 3: trying common post-registration filenames"
  for candidate in \
    process-registration.jsp \
    processregistration.jsp \
    signup-complete.jsp \
    registration-complete.jsp \
    register-complete.jsp \
    complete.jsp \
    account-created.jsp \
    self-registration-complete.jsp; do
      if [ -f "$WEBAPP_DIR/$candidate" ]; then
        TARGET="$WEBAPP_DIR/$candidate"
        log "  picked filename heuristic: $TARGET"
        break
      fi
  done
fi

# ---- if still nothing, give up but list what's there ---------------------
if [ -z "$TARGET" ]; then
  log "ERROR: no patch target found. JSPs available at $WEBAPP_DIR:"
  find "$WEBAPP_DIR" -maxdepth 2 -name '*.jsp' -printf '  %f\n' 2>/dev/null
  log "Manually identify the success-page JSP and rerun this script with"
  log "  SIGNUP_REDIRECT_TARGET_JSP=<filename> set in the wso2is service."
  exit 0
fi

# Allow operator override (when our heuristics pick the wrong file).
if [ -n "${SIGNUP_REDIRECT_TARGET_JSP:-}" ]; then
  if [ -f "$WEBAPP_DIR/$SIGNUP_REDIRECT_TARGET_JSP" ]; then
    TARGET="$WEBAPP_DIR/$SIGNUP_REDIRECT_TARGET_JSP"
    log "operator override: using $TARGET"
  else
    log "WARNING: SIGNUP_REDIRECT_TARGET_JSP=$SIGNUP_REDIRECT_TARGET_JSP not found at $WEBAPP_DIR — falling back to auto-detected"
  fi
fi

# ---- patch idempotently --------------------------------------------------
if grep -q "$MARKER" "$TARGET"; then
  log "$TARGET already patched — no-op"
  exit 0
fi

# Inject just after the first <body opener. Using a JavaScript redirect
# in addition to meta-refresh because some WSO2 builds set CSP headers
# that block meta-refresh — JS still runs as long as inline scripts are
# allowed (which they are on the accountrecoveryendpoint webapp).
INJECTION='<body '"$MARKER"'="true"><meta http-equiv="refresh" content="'"$DELAY_SECONDS"';url='"$REDIRECT_URL"'"><script>setTimeout(function(){window.location="'"$REDIRECT_URL"'";},'"$((DELAY_SECONDS * 1000))"');</script><!-- redirect injected by verifiably-go: returns the user to the OIDC client after WSO2 self-registration completes -->'

# Use | as sed delimiter so URL slashes don't trip the parser. Restrict
# to the FIRST <body match.
sed -i "0,/<body/{s|<body|$INJECTION|}" "$TARGET"

if grep -q "$MARKER" "$TARGET"; then
  log "SUCCESS: meta-refresh + JS redirect injected into $TARGET → $REDIRECT_URL"
else
  log "ERROR: sed completed but marker not present in $TARGET — check the JSP for unusual <body syntax"
fi
