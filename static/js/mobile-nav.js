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
})();
