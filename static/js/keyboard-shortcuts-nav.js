/**
 * Global navigation shortcuts (same guards as dashboard). Pages with richer shortcuts load their own handler.
 */
(function () {
  document.addEventListener('keydown', function (e) {
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT') {
      return;
    }
    if (e.ctrlKey || e.metaKey || e.altKey) {
      return;
    }
    var k = e.key.toLowerCase();
    if (k === 'n' || k === '+') {
      window.location.href = '/manga/add_work';
    } else if (k === 's') {
      window.location.href = '/manga/stats';
    } else if (k === 'p') {
      window.location.href = '/profile';
    } else if (k === 'u') {
      window.location.href = '/manga/users';
    } else if (k === 'g') {
      window.location.href = '/manga/dashboard';
    } else if (k === 'o') {
      window.location.href = '/tools';
    } else if (k === 't' && typeof window.toggleTheme === 'function') {
      e.preventDefault();
      window.toggleTheme();
    }
  });
})();
