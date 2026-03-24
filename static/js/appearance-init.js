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
