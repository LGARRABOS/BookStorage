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
 *
 * Session watch: redirect idle tabs to /login when the sliding session expires,
 * and intercept API 401 responses so users are not stuck on stale HTML.
 */
(function () {
  if (window.__bookstorageDbWatch) return;
  window.__bookstorageDbWatch = true;

  var reloadTriggered = false;
  var sessionRedirectTriggered = false;

  function safeReload() {
    if (reloadTriggered) return;
    reloadTriggered = true;
    window.location.reload();
  }

  function hasSessionCookie() {
    return document.cookie.split(';').some(function (part) {
      var trimmed = part.trim();
      return trimmed.indexOf('session=') === 0 && trimmed.length > 'session='.length;
    });
  }

  function isAuthPage() {
    var path = window.location.pathname || '/';
    return path === '/login' || path === '/register';
  }

  function redirectSessionExpired() {
    if (sessionRedirectTriggered || isAuthPage()) return;
    sessionRedirectTriggered = true;
    window.location.href = '/login?expired=1';
  }

  function requestLooksLikeAPI(input) {
    try {
      var raw = typeof input === 'string' ? input : (input && input.url ? input.url : '');
      if (!raw) return false;
      if (raw.indexOf('http') === 0) {
        return new URL(raw).pathname.indexOf('/api/') === 0;
      }
      return raw.charAt(0) === '/' ? raw.indexOf('/api/') === 0 : raw.indexOf('/api/') !== -1;
    } catch (e) {
      return false;
    }
  }

  var origFetch = window.fetch;
  if (typeof origFetch === 'function') {
    window.fetch = function (input) {
      return origFetch.apply(this, arguments).then(function (res) {
        if (res.status === 401 && hasSessionCookie() && requestLooksLikeAPI(input)) {
          redirectSessionExpired();
          return res;
        }
        if (res.status !== 503) return res;
        var ct = (res.headers.get('content-type') || '').toLowerCase();
        if (ct.indexOf('application/json') === -1) return res;
        return res.clone().json().then(function (j) {
          if (j && j.error === 'service_unavailable' && j.reason === 'database') {
            safeReload();
          }
          return res;
        }).catch(function () { return res; });
      });
    };
  }

  function startPoll() {
    if (document.body && document.body.classList.contains('error-page')) return;
    var pollMs = 10000;
    var failures = 0;
    setInterval(function () {
      if (!origFetch || reloadTriggered) return;
      origFetch('/healthz', {
        credentials: 'same-origin',
        headers: { Accept: 'application/json' }
      })
        .then(function (r) {
          if (!r.ok) {
            failures++;
            if (failures >= 3) safeReload();
            return null;
          }
          failures = 0;
          return r.json();
        })
        .then(function (j) {
          if (j && j.ok === false) {
            failures++;
            if (failures >= 3) safeReload();
          }
        })
        .catch(function () {
          failures++;
          if (failures >= 3) safeReload();
        });
    }, pollMs);
  }

  function startSessionPoll() {
    if (!hasSessionCookie() || isAuthPage()) return;
    if (document.body && document.body.classList.contains('error-page')) return;

    var WARN_BEFORE_MS = 60000;
    var FAST_POLL_MS = 2000;
    var NORMAL_POLL_MS = 60000;
    var APPROACHING_MS = 90000;

    var sessionPollTimer = null;
    var sessionExpiresAtMs = null;
    var warnMessageTpl = '';
    var countdownInterval = null;
    var expireTimeout = null;
    var warnTimeout = null;

    function scheduleNextPoll(delayMs) {
      if (sessionPollTimer) clearTimeout(sessionPollTimer);
      sessionPollTimer = setTimeout(checkSession, delayMs);
    }

    function formatWarnMessage(seconds) {
      if (!warnMessageTpl) {
        return 'Session expires in ' + seconds + ' s';
      }
      return warnMessageTpl.replace('%s', String(seconds));
    }

    function ensureWarningBanner() {
      var banner = document.getElementById('session-expiry-banner');
      if (banner) return banner;
      banner = document.createElement('div');
      banner.id = 'session-expiry-banner';
      banner.className = 'session-expiry-banner';
      banner.hidden = true;
      var inner = document.createElement('p');
      inner.className = 'session-expiry-banner__text';
      banner.appendChild(inner);
      document.body.appendChild(banner);
      return banner;
    }

    function hideSessionWarning() {
      var banner = document.getElementById('session-expiry-banner');
      if (banner) banner.hidden = true;
    }

    function showSessionWarning() {
      var banner = ensureWarningBanner();
      banner.hidden = false;
      banner.setAttribute('role', 'alert');

      function updateCountdown() {
        if (!sessionExpiresAtMs) return;
        var sec = Math.max(0, Math.ceil((sessionExpiresAtMs - Date.now()) / 1000));
        var textEl = banner.querySelector('.session-expiry-banner__text');
        if (textEl) textEl.textContent = formatWarnMessage(sec);
        if (sec <= 0) {
          if (countdownInterval) clearInterval(countdownInterval);
          countdownInterval = null;
          redirectSessionExpired();
        }
      }

      updateCountdown();
      if (countdownInterval) clearInterval(countdownInterval);
      countdownInterval = setInterval(updateCountdown, 1000);
    }

    function armSessionTimers() {
      if (expireTimeout) clearTimeout(expireTimeout);
      if (warnTimeout) clearTimeout(warnTimeout);
      if (countdownInterval) clearInterval(countdownInterval);
      countdownInterval = null;

      if (!sessionExpiresAtMs) return;
      var remaining = sessionExpiresAtMs - Date.now();
      if (remaining <= 0) {
        redirectSessionExpired();
        return;
      }

      var warnIn = remaining - WARN_BEFORE_MS;
      if (warnIn <= 0) {
        showSessionWarning();
      } else {
        hideSessionWarning();
        warnTimeout = setTimeout(showSessionWarning, warnIn);
      }

      expireTimeout = setTimeout(function () {
        if (!origFetch) {
          redirectSessionExpired();
          return;
        }
        origFetch('/api/session/ping', {
          credentials: 'same-origin',
          headers: { Accept: 'application/json' }
        }).finally(function () {
          redirectSessionExpired();
        });
      }, remaining);
    }

    function checkSession() {
      if (!origFetch || sessionRedirectTriggered || reloadTriggered) return;
      if (!hasSessionCookie() || isAuthPage()) return;

      origFetch('/api/session/ping', {
        credentials: 'same-origin',
        headers: { Accept: 'application/json' }
      })
        .then(function (r) {
          if (r.status === 401) {
            redirectSessionExpired();
            return null;
          }
          if (!r.ok) return null;
          return r.json();
        })
        .then(function (j) {
          if (!j || !j.ok) {
            scheduleNextPoll(NORMAL_POLL_MS);
            return;
          }
          if (j.warn_message) warnMessageTpl = j.warn_message;
          var expiresMs = Date.parse(j.expires_at);
          if (isNaN(expiresMs)) {
            scheduleNextPoll(NORMAL_POLL_MS);
            return;
          }
          sessionExpiresAtMs = expiresMs;
          armSessionTimers();
          var remaining = expiresMs - Date.now();
          scheduleNextPoll(remaining <= APPROACHING_MS ? FAST_POLL_MS : NORMAL_POLL_MS);
        })
        .catch(function () {
          scheduleNextPoll(NORMAL_POLL_MS);
        });
    }

    checkSession();
  }

  function startWatchers() {
    startPoll();
    startSessionPoll();
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', startWatchers);
  } else {
    startWatchers();
  }
})();
