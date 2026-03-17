/**
 * Custom modals to replace native alert() and confirm()
 * Usage: showConfirm(msg).then(ok => { if (ok) ... })
 *        showAlert(msg).then(() => ...)
 */
(function() {
  function ensureModals() {
    if (document.getElementById('confirm-modal')) return;
    const confirmHtml = '<div id="confirm-modal" class="confirm-modal-overlay" aria-hidden="true">' +
      '<div class="confirm-modal-backdrop"></div>' +
      '<div class="confirm-modal-dialog">' +
      '<h3 class="confirm-modal-title" id="confirm-modal-title"></h3>' +
      '<p class="confirm-modal-msg" id="confirm-modal-msg"></p>' +
      '<div class="confirm-modal-actions">' +
      '<button type="button" class="btn btn-secondary" id="confirm-modal-cancel"></button>' +
      '<button type="button" class="btn btn-danger" id="confirm-modal-ok"></button>' +
      '</div></div></div>';
    const alertHtml = '<div id="alert-modal" class="alert-modal-overlay" aria-hidden="true">' +
      '<div class="alert-modal-backdrop"></div>' +
      '<div class="alert-modal-dialog">' +
      '<h3 class="alert-modal-title" id="alert-modal-title"></h3>' +
      '<p class="alert-modal-msg" id="alert-modal-msg"></p>' +
      '<div class="alert-modal-actions">' +
      '<button type="button" class="btn btn-primary" id="alert-modal-ok"></button>' +
      '</div></div></div>';
    document.body.insertAdjacentHTML('beforeend', confirmHtml + alertHtml);
  }

  window.showConfirm = function(message, options) {
    ensureModals();
    const opts = options || {};
    const title = opts.title || (document.documentElement.lang === 'fr' ? 'Confirmer' : 'Confirm');
    const okLabel = opts.okLabel || (document.documentElement.lang === 'fr' ? 'Supprimer' : 'Delete');
    const cancelLabel = opts.cancelLabel || (document.documentElement.lang === 'fr' ? 'Annuler' : 'Cancel');

    const modal = document.getElementById('confirm-modal');
    const msgEl = document.getElementById('confirm-modal-msg');
    const titleEl = document.getElementById('confirm-modal-title');
    const cancelBtn = document.getElementById('confirm-modal-cancel');
    const okBtn = document.getElementById('confirm-modal-ok');

    titleEl.textContent = title;
    msgEl.textContent = message;
    cancelBtn.textContent = cancelLabel;
    okBtn.textContent = okLabel;

    return new Promise(function(resolve) {
      function close(result) {
        modal.setAttribute('aria-hidden', 'true');
        modal.style.display = 'none';
        cancelBtn.onclick = null;
        okBtn.onclick = null;
        okBtn.onkeydown = null;
        cancelBtn.onkeydown = null;
        modal.onkeydown = null;
        resolve(result);
      }
      cancelBtn.onclick = function() { close(false); };
      okBtn.onclick = function() { close(true); };
      const confirmBackdrop = modal.querySelector('.confirm-modal-backdrop');
      if (confirmBackdrop) confirmBackdrop.onclick = function() { close(false); };
      function onKey(e) {
        if (e.key === 'Escape') { e.preventDefault(); close(false); }
      }
      modal.onkeydown = onKey;
      cancelBtn.onkeydown = okBtn.onkeydown = onKey;

      modal.setAttribute('aria-hidden', 'false');
      modal.style.display = 'flex';
      okBtn.focus();
    });
  };

  window.showAlert = function(message, options) {
    ensureModals();
    const opts = options || {};
    const title = opts.title || (document.documentElement.lang === 'fr' ? 'Information' : 'Information');
    const okLabel = opts.okLabel || 'OK';

    const modal = document.getElementById('alert-modal');
    const msgEl = document.getElementById('alert-modal-msg');
    const titleEl = document.getElementById('alert-modal-title');
    const okBtn = document.getElementById('alert-modal-ok');

    titleEl.textContent = title;
    msgEl.textContent = message;
    okBtn.textContent = okLabel;

    return new Promise(function(resolve) {
      function close() {
        modal.setAttribute('aria-hidden', 'true');
        modal.style.display = 'none';
        okBtn.onclick = null;
        okBtn.onkeydown = null;
        modal.onkeydown = null;
        resolve();
      }
      okBtn.onclick = close;
      const alertBackdrop = modal.querySelector('.alert-modal-backdrop');
      if (alertBackdrop) alertBackdrop.onclick = close;
      function onKey(e) {
        if (e.key === 'Escape' || e.key === 'Enter') { e.preventDefault(); close(); }
      }
      modal.onkeydown = onKey;
      okBtn.onkeydown = onKey;

      modal.setAttribute('aria-hidden', 'false');
      modal.style.display = 'flex';
      okBtn.focus();
    });
  };
})();
