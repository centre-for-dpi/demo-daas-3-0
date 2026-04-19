// app.js — minimal client-side behaviour.
// HTMX handles all async server interaction; this file only covers
// concerns that genuinely need the browser: toasts, theme, clipboard,
// and surfacing unhandled server errors.

(function () {
  'use strict';

  // ---- Theme toggle ----
  const THEME_KEY = 'verifiably_theme';
  function applyTheme(t) {
    document.documentElement.setAttribute('data-theme', t);
    try { localStorage.setItem(THEME_KEY, t); } catch (e) { /* private mode */ }
  }
  window.toggleTheme = function () {
    const cur = document.documentElement.getAttribute('data-theme') || 'light';
    applyTheme(cur === 'light' ? 'dark' : 'light');
  };
  try {
    const saved = localStorage.getItem(THEME_KEY);
    if (saved) applyTheme(saved);
  } catch (e) {}

  // ---- Toast ----
  let toastTimer = null;
  function toast(msg) {
    const el = document.getElementById('toast');
    if (!el) return;
    el.textContent = msg;
    el.classList.add('show');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => el.classList.remove('show'), 2800);
  }
  window.toast = toast;

  // ---- HTMX event bindings ----
  // HX-Trigger: toast:<message>  →  server-initiated toast
  document.body.addEventListener('htmx:afterOnLoad', function (evt) {
    // older htmx sets HX-Trigger as a custom event; we also listen below.
  });

  // HTMX triggers custom events from HX-Trigger. Listen for both the
  // single-form (plain string "toast:msg") and the JSON form.
  document.body.addEventListener('htmx:trigger', function (evt) {
    // noop — individual events below handle specifics
  });

  // Server sends: HX-Trigger: toast:Some message  → htmx dispatches `toast:Some message` event
  // but because htmx treats the text after the colon as the event name, we also
  // accept the JSON form: HX-Trigger: {"toast":"Some message"}
  document.body.addEventListener('toast', function (evt) {
    const msg = (evt.detail && (evt.detail.value || evt.detail)) || evt.detail || '';
    if (typeof msg === 'string') toast(msg);
    else if (msg && msg.value) toast(msg.value);
  });

  // Error surface — if an HTMX request fails, show a toast instead of a silent failure.
  document.body.addEventListener('htmx:responseError', function (evt) {
    const status = evt.detail && evt.detail.xhr && evt.detail.xhr.status;
    toast('Server error' + (status ? ' (' + status + ')' : ''));
  });
  document.body.addEventListener('htmx:sendError', function () {
    toast('Network error — check your connection');
  });

  // ---- Multipart upload helper (verifier QR image upload) ----
  // HTMX 2.x multipart submission is finicky on <form>-level hx-post; we drive
  // the POST directly and swap the result into #verify-result ourselves.
  window.uploadQR = function (evt) {
    evt.preventDefault();
    const form = evt.target;
    const action = form.getAttribute('hx-post') || '/verifier/verify/direct';
    const data = new FormData(form);
    fetch(action, { method: 'POST', body: data, headers: { 'HX-Request': 'true' } })
      .then((r) => r.text())
      .then((html) => {
        const tgt = document.getElementById('verify-result');
        if (tgt) tgt.innerHTML = html;
      })
      .catch((err) => toast('Upload failed: ' + err.message));
    return false;
  };

  // ---- Clipboard helper (exposed for onclick attributes) ----
  window.copyText = function (text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(
        () => toast('Copied to clipboard'),
        () => toast('Copy failed — select manually')
      );
    } else {
      toast('Clipboard not available');
    }
  };
})();
