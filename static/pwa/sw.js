// build: 20260617-3
const CACHE_NAME = 'bookstorage-v13';
const STATIC_ASSETS = [
  '/static/css/base.css',
  '/static/css/brand.css',
  '/static/css/mobile.css',
  '/static/css/dashboard-mobile.css',
  '/static/css/work-status-picker.css',
  '/static/js/appearance-init.js',
  '/static/js/appearance.js',
  '/static/js/modals.js',
  '/static/js/mobile-shell.js',
  '/static/js/mobile-nav.js',
  '/static/js/mobile-filters.js',
  '/static/js/mobile-dashboard.js',
  '/static/js/work-status-picker.js',
  '/static/pwa/manifest.json',
  '/static/pwa/offline.html',
  '/static/brand/pwa/icon-192.png',
  '/static/brand/pwa/icon-512.png',
  '/static/brand/favicon/favicon-32.png',
  '/static/brand/favicon/favicon-16.png',
  '/static/brand/logos/logo.png',
  '/static/brand/banners/banners.png',
];

self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_NAME).then(cache => {
      return cache.addAll(STATIC_ASSETS);
    })
  );
  self.skipWaiting();
});

self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys().then(keys => {
      return Promise.all(
        keys.filter(key => key !== CACHE_NAME).map(key => caches.delete(key))
      );
    })
  );
  self.clients.claim();
});

function isCacheableStatic(url) {
  if (!url.includes('/static/')) return false;
  if (url.includes('/static/images/') || url.includes('/static/avatars/')) return false;
  return true;
}

function isDocumentRequest(request) {
  return request.mode === 'navigate' ||
    (request.method === 'GET' && request.headers.get('accept') && request.headers.get('accept').includes('text/html'));
}

self.addEventListener('fetch', event => {
  if (event.request.method !== 'GET') return;
  if (event.request.url.includes('/api/')) return;

  if (isDocumentRequest(event.request)) {
    event.respondWith(
      fetch(event.request)
        .then(response => response)
        .catch(() => caches.match('/static/pwa/offline.html'))
    );
    return;
  }

  event.respondWith(
    fetch(event.request)
      .then(response => {
        if (response.ok && isCacheableStatic(event.request.url)) {
          const responseClone = response.clone();
          caches.open(CACHE_NAME).then(cache => {
            cache.put(event.request, responseClone);
          });
        }
        return response;
      })
      .catch(() => caches.match(event.request))
  );
});
