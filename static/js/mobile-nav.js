(function () {
  function init() {
    var toggle = document.querySelector('[data-mobile-menu-toggle]');
    var menu = document.querySelector('[data-mobile-menu]');
    if (toggle && menu) {
      toggle.addEventListener('click', function () {
        var isOpen = menu.classList.toggle('is-open');
        toggle.setAttribute('aria-expanded', isOpen ? 'true' : 'false');
      });
    }

    var current = window.location.pathname;
    var links = document.querySelectorAll('.mobile-bottom-link[data-nav]');
    for (var i = 0; i < links.length; i += 1) {
      var href = links[i].getAttribute('data-nav');
      if (!href) continue;
      if (current === href || (href !== '/dashboard' && current.indexOf(href) === 0)) {
        links[i].classList.add('is-active');
      }
    }

    var langToggle = document.querySelector('[data-lang-burger-toggle]');
    var langPanel = document.querySelector('[data-lang-burger-panel]');
    var langRoot = document.querySelector('[data-lang-burger]');
    if (langToggle && langPanel && langRoot) {
      langToggle.addEventListener('click', function (e) {
        e.stopPropagation();
        var willShow = langPanel.hasAttribute('hidden');
        if (willShow) {
          langPanel.removeAttribute('hidden');
          langToggle.setAttribute('aria-expanded', 'true');
        } else {
          langPanel.setAttribute('hidden', '');
          langToggle.setAttribute('aria-expanded', 'false');
        }
      });
      document.addEventListener(
        'click',
        function (e) {
          if (langPanel.hasAttribute('hidden')) return;
          if (langRoot.contains(e.target)) return;
          langPanel.setAttribute('hidden', '');
          langToggle.setAttribute('aria-expanded', 'false');
        },
        false
      );
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
