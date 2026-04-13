(function () {
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
  if (langToggle && langPanel) {
    langToggle.addEventListener('click', function (e) {
      e.stopPropagation();
      var open = langPanel.hasAttribute('hidden');
      if (open) {
        langPanel.removeAttribute('hidden');
        langToggle.setAttribute('aria-expanded', 'true');
      } else {
        langPanel.setAttribute('hidden', '');
        langToggle.setAttribute('aria-expanded', 'false');
      }
    });
    document.addEventListener('click', function (e) {
      if (!langPanel.hasAttribute('hidden') && !langPanel.contains(e.target) && e.target !== langToggle) {
        langPanel.setAttribute('hidden', '');
        langToggle.setAttribute('aria-expanded', 'false');
      }
    });
  }
})();
