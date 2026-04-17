/**
 * Apply appearance preferences before first paint. Keep in sync with appearance.js.
 */
(function () {
  var html = document.documentElement;

  var theme = localStorage.getItem('theme') || 'light';
  if (theme !== 'light' && theme !== 'dark') theme = 'light';
  html.setAttribute('data-theme', theme);

  var textSize = localStorage.getItem('textSize') || 'md';
  if (textSize !== 'sm' && textSize !== 'md' && textSize !== 'lg') textSize = 'md';
  html.setAttribute('data-text-size', textSize);

  var motionStored = localStorage.getItem('motion');
  var motion;
  if (motionStored === null || motionStored === '') {
    motion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
      ? 'reduce'
      : 'no-preference';
  } else if (motionStored === 'reduce' || motionStored === 'no-preference') {
    motion = motionStored;
  } else {
    motion = 'no-preference';
  }
  html.setAttribute('data-motion', motion);

  var contrast = localStorage.getItem('contrast') || 'normal';
  if (contrast !== 'more') contrast = 'normal';
  html.setAttribute('data-contrast', contrast);

  var scrollbars = localStorage.getItem('scrollbars') || 'hidden';
  if (scrollbars !== 'visible') scrollbars = 'hidden';
  html.setAttribute('data-scrollbars', scrollbars);
})();

/**
 * When the database drops, full-page navigations are blocked by the server (503),
 * but in-page fetch() calls still need a reload to show the maintenance HTML.
 * Also poll /healthz so idle tabs detect outage without user action.
 */
(function () {
  if (window.__bookstorageDbWatch) return;
  window.__bookstorageDbWatch = true;

  var origFetch = window.fetch;
  if (typeof origFetch === 'function') {
    window.fetch = function () {
      return origFetch.apply(this, arguments).then(function (res) {
        if (res.status !== 503) return res;
        var ct = (res.headers.get('content-type') || '').toLowerCase();
        if (ct.indexOf('application/json') === -1) return res;
        return res.clone().json().then(function (j) {
          if (j && j.error === 'service_unavailable' && j.reason === 'database') {
            window.location.reload();
          }
          return res;
        }).catch(function () {
          window.location.reload();
          return res;
        });
      });
    };
  }

  function startPoll() {
    if (document.body && document.body.classList.contains('error-page')) return;
    var pollMs = 5000;
    setInterval(function () {
      if (!origFetch) return;
      origFetch('/healthz', {
        credentials: 'same-origin',
        headers: { Accept: 'application/json' }
      })
        .then(function (r) {
          if (!r.ok) {
            window.location.reload();
            return null;
          }
          return r.json();
        })
        .then(function (j) {
          if (j && j.ok === false) window.location.reload();
        })
        .catch(function () {
          window.location.reload();
        });
    }, pollMs);
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', startPoll);
  } else {
    startPoll();
  }
})();
