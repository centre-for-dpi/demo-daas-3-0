/* ========================================
   app.js — Minimal vanilla JS
   Only what HTMX can't do.
   ======================================== */

/* ---- Capability matrix bootstrapper ----
   Fetches /api/capabilities once on page load and stores the result in
   window._capabilities. Drives sidebar visibility, beta banners, and any
   other per-DPG UI variations. Re-runs on htmx:afterSettle so portal navigations
   keep the UI consistent. */
window._capabilities = null;
window._capabilitiesLoaded = false;

function applyCapabilities() {
  var caps = window._capabilities;
  if (!caps) return;

  // Beta banners — show a small badge next to the sidebar section header
  // for any service whose backend is in beta. Banner is also shown inline
  // on the relevant pages.
  document.querySelectorAll('[data-dpg-section]').forEach(function(el) {
    var section = el.getAttribute('data-dpg-section');
    var beta = false;
    if (section === 'issuer' && caps.issuerBeta) beta = true;
    if (section === 'wallet' && caps.walletBeta) beta = true;
    if (section === 'verifier' && caps.verifierBeta) beta = true;
    if (beta) {
      el.classList.add('dpg-beta');
      // Add inline beta tag if not already present
      if (!el.querySelector('.dpg-beta-tag')) {
        var tag = document.createElement('span');
        tag.className = 'dpg-beta-tag';
        tag.textContent = 'BETA';
        tag.style.cssText = 'margin-left:0.5rem;font-size:0.55rem;padding:0.1rem 0.35rem;background:var(--warning-surface);color:var(--warning);border-radius:0.2rem;font-weight:600';
        el.appendChild(tag);
      }
    } else {
      el.classList.remove('dpg-beta');
      var existing = el.querySelector('.dpg-beta-tag');
      if (existing) existing.remove();
    }
  });

  // Hide flows the active backend doesn't support.
  // Sidebar items can declare data-cap="issuer.batch" and they'll be hidden
  // when caps.issuer.batch is false.
  document.querySelectorAll('[data-cap]').forEach(function(el) {
    var path = el.getAttribute('data-cap').split('.');
    var v = caps;
    for (var i = 0; i < path.length; i++) {
      if (v && typeof v === 'object') v = v[path[i]];
      else { v = false; break; }
    }
    if (!v) {
      el.style.display = 'none';
    } else {
      el.style.display = '';
    }
  });

  // Update topbar DPG badges if present
  var issuerBadge = document.getElementById('dpg-issuer-name');
  if (issuerBadge) issuerBadge.textContent = caps.issuerName || '';
  var walletBadge = document.getElementById('dpg-wallet-name');
  if (walletBadge) walletBadge.textContent = caps.walletName || '';
  var verifierBadge = document.getElementById('dpg-verifier-name');
  if (verifierBadge) verifierBadge.textContent = caps.verifierName || '';
}

function loadCapabilities() {
  fetch('/api/capabilities')
    .then(function(r) { return r.json(); })
    .then(function(caps) {
      window._capabilities = caps;
      window._capabilitiesLoaded = true;
      applyCapabilities();
    })
    .catch(function() { /* silent — caps stay null, UI behaves as default */ });
}

// Load on initial page load
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', loadCapabilities);
} else {
  loadCapabilities();
}

// Re-apply on every HTMX content swap (sidebar may have new items)
document.addEventListener('htmx:afterSettle', function() {
  if (window._capabilities) applyCapabilities();
});


/* ---- Theme toggle ---- */
function toggleTheme() {
  const html = document.documentElement;
  const current = html.getAttribute('data-theme');
  const next = current === 'light' ? 'dark' : 'light';
  html.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
}

// Restore saved theme on load
(function () {
  const saved = localStorage.getItem('theme');
  if (saved) {
    document.documentElement.setAttribute('data-theme', saved);
  }
})();

/* ---- Toast system (driven by HTMX HX-Trigger) ---- */
document.addEventListener('showToast', function (e) {
  var detail = e.detail || {};
  showToast(detail.title || 'Done', detail.text || '');
});

function showToast(title, text) {
  var container = document.getElementById('toast-container');
  if (!container) return;

  var toast = document.createElement('div');
  toast.className = 'toast';
  toast.innerHTML =
    '<span class="toast-icon">\u2713</span>' +
    '<div class="toast-text"><strong>' + escapeHtml(title) + '</strong>' +
    (text ? '<br><span style="font-size:0.78rem;color:var(--text-secondary)">' + escapeHtml(text) + '</span>' : '') +
    '</div>' +
    '<button class="toast-close" onclick="this.parentElement.remove()">\u2715</button>';

  container.appendChild(toast);
  setTimeout(function () { toast.remove(); }, 5000);
}

function escapeHtml(str) {
  var div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

/* ---- User menu dropdown ---- */
function toggleUserMenu() {
  var menu = document.getElementById('user-menu');
  if (menu) menu.classList.toggle('open');
}

// Close user menu on outside click
document.addEventListener('click', function (e) {
  var menu = document.getElementById('user-menu');
  var trigger = document.getElementById('user-menu-trigger');
  if (menu && trigger && !trigger.contains(e.target) && !menu.contains(e.target)) {
    menu.classList.remove('open');
  }
});

/* ---- Language / Translation (DeepL) ---- */
var _currentLang = localStorage.getItem('lang') || 'EN';
var _translationCache = {}; // key: "lang:text" -> translated
var _isTranslating = false;

(function initLang() {
  var sel = document.getElementById('lang-select');
  if (sel) sel.value = _currentLang;
  if (_currentLang !== 'EN') {
    // Translate on initial load after a short delay
    setTimeout(function() { translatePage(_currentLang); }, 300);
  }
})();

function switchLanguage(lang) {
  _currentLang = lang;
  localStorage.setItem('lang', lang);
  if (lang === 'EN') {
    // Restore original English text
    restoreOriginals();
    return;
  }
  translatePage(lang);
}

function translatePage(lang) {
  if (_isTranslating) return;
  var nodes = getTranslatableNodes(document.body);
  if (nodes.length === 0) return;

  // Apply cached translations immediately
  var uncached = [];
  var uncachedNodes = [];
  for (var i = 0; i < nodes.length; i++) {
    var original = nodes[i]._originalText || nodes[i].textContent.trim();
    if (!nodes[i]._originalText) nodes[i]._originalText = original;
    if (!original) continue;

    var cacheKey = lang + ':' + original;
    if (_translationCache[cacheKey]) {
      nodes[i].textContent = _translationCache[cacheKey];
    } else {
      uncached.push(original);
      uncachedNodes.push(nodes[i]);
    }
  }

  if (uncached.length === 0) return;

  // Deduplicate
  var unique = [];
  var uniqueMap = {};
  for (var i = 0; i < uncached.length; i++) {
    if (!uniqueMap[uncached[i]]) {
      uniqueMap[uncached[i]] = true;
      unique.push(uncached[i]);
    }
  }

  // Send in progressive batches of 30 for faster first paint
  _isTranslating = true;
  var BATCH = 30;
  var batches = [];
  for (var i = 0; i < unique.length; i += BATCH) {
    batches.push(unique.slice(i, i + BATCH));
  }

  var done = 0;
  batches.forEach(function(batch) {
    fetch('/api/translate', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({texts: batch, target: lang})
    })
    .then(function(r) { return r.json(); })
    .then(function(d) {
      if (d.translations) {
        for (var i = 0; i < batch.length && i < d.translations.length; i++) {
          _translationCache[lang + ':' + batch[i]] = d.translations[i];
        }
        // Apply to matching nodes
        for (var i = 0; i < uncachedNodes.length; i++) {
          var orig = uncachedNodes[i]._originalText;
          if (orig && _translationCache[lang + ':' + orig]) {
            uncachedNodes[i].textContent = _translationCache[lang + ':' + orig];
          }
        }
      }
    })
    .catch(function(){})
    .finally(function() {
      done++;
      if (done >= batches.length) _isTranslating = false;
    });
  });
}

function restoreOriginals() {
  var nodes = getTranslatableNodes(document.body);
  for (var i = 0; i < nodes.length; i++) {
    if (nodes[i]._originalText) {
      nodes[i].textContent = nodes[i]._originalText;
    }
  }
}

function getTranslatableNodes(root) {
  if (!root) return [];
  var result = [];
  var walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode: function(node) {
      var text = node.textContent.trim();
      if (!text || text.length < 2) return NodeFilter.FILTER_REJECT;
      // Skip code/pre/script/style elements
      var parent = node.parentElement;
      if (!parent) return NodeFilter.FILTER_REJECT;
      var tag = parent.tagName;
      if (tag === 'SCRIPT' || tag === 'STYLE' || tag === 'CODE' || tag === 'PRE' || tag === 'TEXTAREA' || tag === 'INPUT') {
        return NodeFilter.FILTER_REJECT;
      }
      // Skip if text looks like a URL, DID, or technical identifier
      if (text.indexOf('://') !== -1 || text.indexOf('did:') === 0 || text.match(/^[a-f0-9-]{20,}$/)) {
        return NodeFilter.FILTER_REJECT;
      }
      return NodeFilter.FILTER_ACCEPT;
    }
  });
  while (walker.nextNode()) {
    result.push(walker.currentNode);
  }
  return result;
}

/* ---- Sidebar active state + re-translate on HTMX swap ---- */
document.addEventListener('htmx:afterSettle', function () {
  // Re-translate new content if not in English
  if (_currentLang && _currentLang !== 'EN') {
    setTimeout(function() { translatePage(_currentLang); }, 100);
  }
  // Restore language selector value (topbar may have been re-rendered)
  var sel = document.getElementById('lang-select');
  if (sel && sel.value !== _currentLang) sel.value = _currentLang;
  var path = window.location.pathname;
  var bestMatch = null;
  var bestLen = 0;

  // Find the sidebar item whose hx-get is the longest prefix match for the current path.
  // This ensures /portal/issuer/schemas matches "Issuer" (/portal/issuer/schemas)
  // over "Dashboard" (/portal/dashboard).
  document.querySelectorAll('.sidebar-item[hx-get]').forEach(function (item) {
    var href = item.getAttribute('hx-get');
    if (href && path.indexOf(href) === 0 && href.length > bestLen) {
      bestMatch = item;
      bestLen = href.length;
    }
  });

  document.querySelectorAll('.sidebar-item').forEach(function (item) {
    item.classList.remove('active');
  });
  if (bestMatch) bestMatch.classList.add('active');
});
