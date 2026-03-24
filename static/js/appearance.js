/**
 * Theme and accessibility preferences (localStorage). Requires appearance-init.js in <head>.
 */
(function () {
  function getHtml() {
    return document.documentElement;
  }

  function updateThemeIcon() {
    var theme = getHtml().getAttribute('data-theme') || 'light';
    var icons = document.querySelectorAll('#theme-icon');
    for (var i = 0; i < icons.length; i++) {
      icons[i].textContent = theme === 'dark' ? '☀️' : '🌙';
    }
    var toggles = document.querySelectorAll('.theme-toggle');
    for (var j = 0; j < toggles.length; j++) {
      toggles[j].setAttribute('aria-pressed', theme === 'dark' ? 'true' : 'false');
    }
  }

  window.toggleTheme = function () {
    var html = getHtml();
    var next = (html.getAttribute('data-theme') || 'light') === 'light' ? 'dark' : 'light';
    html.setAttribute('data-theme', next);
    localStorage.setItem('theme', next);
    updateThemeIcon();
  };

  window.BookStorageAppearance = {
    setTextSize: function (v) {
      if (v !== 'sm' && v !== 'md' && v !== 'lg') return;
      getHtml().setAttribute('data-text-size', v);
      localStorage.setItem('textSize', v);
      var el = document.getElementById('a11y-text-size');
      if (el) el.value = v;
    },
    setMotion: function (v) {
      var html = getHtml();
      if (v === '' || v === null) {
        localStorage.removeItem('motion');
        var motion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
          ? 'reduce'
          : 'no-preference';
        html.setAttribute('data-motion', motion);
      } else if (v === 'reduce' || v === 'no-preference') {
        localStorage.setItem('motion', v);
        html.setAttribute('data-motion', v);
      }
      var sel = document.getElementById('a11y-motion');
      if (sel) sel.value = v === 'reduce' || v === 'no-preference' ? v : '';
    },
    setContrast: function (on) {
      var v = on ? 'more' : 'normal';
      getHtml().setAttribute('data-contrast', v);
      localStorage.setItem('contrast', v);
      var cb = document.getElementById('a11y-contrast');
      if (cb) cb.checked = on;
    },
    setScrollbarsVisible: function (on) {
      var v = on ? 'visible' : 'hidden';
      getHtml().setAttribute('data-scrollbars', v);
      localStorage.setItem('scrollbars', v);
      var cb = document.getElementById('a11y-scrollbars');
      if (cb) cb.checked = on;
    },
    syncProfileForm: function () {
      var html = getHtml();
      var ts = document.getElementById('a11y-text-size');
      if (ts) ts.value = html.getAttribute('data-text-size') || 'md';

      var motionSel = document.getElementById('a11y-motion');
      if (motionSel) {
        var stored = localStorage.getItem('motion');
        if (stored === null || stored === '') {
          motionSel.value = '';
        } else {
          motionSel.value = stored === 'reduce' ? 'reduce' : 'no-preference';
        }
      }

      var contrastCb = document.getElementById('a11y-contrast');
      if (contrastCb) {
        contrastCb.checked = (html.getAttribute('data-contrast') || 'normal') === 'more';
      }

      var sbCb = document.getElementById('a11y-scrollbars');
      if (sbCb) {
        sbCb.checked = (html.getAttribute('data-scrollbars') || 'hidden') === 'visible';
      }
    },
    init: function () {
      updateThemeIcon();
      if (document.getElementById('a11y-text-size')) {
        window.BookStorageAppearance.syncProfileForm();
        var ts = document.getElementById('a11y-text-size');
        if (ts) {
          ts.addEventListener('change', function () {
            window.BookStorageAppearance.setTextSize(ts.value);
          });
        }
        var motionSel = document.getElementById('a11y-motion');
        if (motionSel) {
          motionSel.addEventListener('change', function () {
            window.BookStorageAppearance.setMotion(motionSel.value);
          });
        }
        var contrastCb = document.getElementById('a11y-contrast');
        if (contrastCb) {
          contrastCb.addEventListener('change', function () {
            window.BookStorageAppearance.setContrast(contrastCb.checked);
          });
        }
        var sbCb = document.getElementById('a11y-scrollbars');
        if (sbCb) {
          sbCb.addEventListener('change', function () {
            window.BookStorageAppearance.setScrollbarsVisible(sbCb.checked);
          });
        }
      }
    }
  };

  document.addEventListener('DOMContentLoaded', function () {
    updateThemeIcon();
    if (window.BookStorageAppearance) {
      window.BookStorageAppearance.init();
    }
  });
})();
