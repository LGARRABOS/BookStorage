/**
 * Custom modals to replace native alert() and confirm()
 * Usage: showConfirm(msg).then(ok => { if (ok) ... })
 *        showAlert(msg).then(() => ...)
 */
(function() {
  function getFocusable(dialog) {
    if (!dialog) return [];
    return Array.prototype.slice.call(
      dialog.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])')
    ).filter(function(el) {
      return !el.hasAttribute('disabled') && el.offsetParent !== null;
    });
  }

  function ensureModals() {
    if (document.getElementById('confirm-modal')) return;
    const confirmHtml = '<div id="confirm-modal" class="confirm-modal-overlay" aria-hidden="true">' +
      '<div class="confirm-modal-backdrop"></div>' +
      '<div class="confirm-modal-dialog" role="dialog" aria-modal="true" aria-labelledby="confirm-modal-title" aria-describedby="confirm-modal-msg" tabindex="-1">' +
      '<h3 class="confirm-modal-title" id="confirm-modal-title"></h3>' +
      '<p class="confirm-modal-msg" id="confirm-modal-msg"></p>' +
      '<div class="confirm-modal-actions">' +
      '<button type="button" class="btn btn-secondary" id="confirm-modal-cancel"></button>' +
      '<button type="button" class="btn btn-danger" id="confirm-modal-ok"></button>' +
      '</div></div></div>';
    const alertHtml = '<div id="alert-modal" class="alert-modal-overlay" aria-hidden="true">' +
      '<div class="alert-modal-backdrop"></div>' +
      '<div class="alert-modal-dialog" role="dialog" aria-modal="true" aria-labelledby="alert-modal-title" aria-describedby="alert-modal-msg" tabindex="-1">' +
      '<h3 class="alert-modal-title" id="alert-modal-title"></h3>' +
      '<p class="alert-modal-msg" id="alert-modal-msg"></p>' +
      '<div class="alert-modal-actions">' +
      '<button type="button" class="btn btn-primary" id="alert-modal-ok"></button>' +
      '</div></div></div>';
    document.body.insertAdjacentHTML('beforeend', confirmHtml + alertHtml);
  }

  function trapFocus(e, dialog, lastFocused) {
    var focusables = getFocusable(dialog);
    if (focusables.length === 0) return;
    var first = focusables[0];
    var last = focusables[focusables.length - 1];
    if (e.key === 'Tab') {
      if (e.shiftKey) {
        if (document.activeElement === first) {
          e.preventDefault();
          last.focus();
        }
      } else {
        if (document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    }
  }

  window.showConfirm = function(message, options) {
    ensureModals();
    var opts = options || {};
    var title = opts.title || (document.documentElement.lang === 'fr' ? 'Confirmer' : 'Confirm');
    var okLabel = opts.okLabel || (document.documentElement.lang === 'fr' ? 'Supprimer' : 'Delete');
    var cancelLabel = opts.cancelLabel || (document.documentElement.lang === 'fr' ? 'Annuler' : 'Cancel');
    var okClass = opts.okClass || 'btn-danger';

    var modal = document.getElementById('confirm-modal');
    var dialogEl = modal.querySelector('.confirm-modal-dialog');
    var msgEl = document.getElementById('confirm-modal-msg');
    var titleEl = document.getElementById('confirm-modal-title');
    var cancelBtn = document.getElementById('confirm-modal-cancel');
    var okBtn = document.getElementById('confirm-modal-ok');

    titleEl.textContent = title;
    msgEl.textContent = message;
    cancelBtn.textContent = cancelLabel;
    okBtn.textContent = okLabel;
    okBtn.className = 'btn ' + okClass;

    var previousActive = document.activeElement;

    return new Promise(function(resolve) {
      function close(result) {
        modal.setAttribute('aria-hidden', 'true');
        modal.style.display = 'none';
        cancelBtn.onclick = null;
        okBtn.onclick = null;
        modal.removeEventListener('keydown', onTrap);
        modal.onkeydown = null;
        if (previousActive && typeof previousActive.focus === 'function') {
          previousActive.focus();
        }
        resolve(result);
      }
      function onTrap(e) {
        if (e.key === 'Escape') {
          e.preventDefault();
          close(false);
          return;
        }
        trapFocus(e, dialogEl);
      }
      cancelBtn.onclick = function() { close(false); };
      okBtn.onclick = function() { close(true); };
      var confirmBackdrop = modal.querySelector('.confirm-modal-backdrop');
      if (confirmBackdrop) confirmBackdrop.onclick = function() { close(false); };
      modal.addEventListener('keydown', onTrap);

      modal.setAttribute('aria-hidden', 'false');
      modal.style.display = 'flex';
      okBtn.focus();
    });
  };

  window.showAlert = function(message, options) {
    ensureModals();
    var opts = options || {};
    var title = opts.title || (document.documentElement.lang === 'fr' ? 'Information' : 'Information');
    var okLabel = opts.okLabel || 'OK';

    var modal = document.getElementById('alert-modal');
    var dialogEl = modal.querySelector('.alert-modal-dialog');
    var msgEl = document.getElementById('alert-modal-msg');
    var titleEl = document.getElementById('alert-modal-title');
    var okBtn = document.getElementById('alert-modal-ok');

    titleEl.textContent = title;
    msgEl.textContent = message;
    okBtn.textContent = okLabel;

    var previousActive = document.activeElement;

    return new Promise(function(resolve) {
      function close() {
        modal.setAttribute('aria-hidden', 'true');
        modal.style.display = 'none';
        okBtn.onclick = null;
        modal.removeEventListener('keydown', onTrap);
        modal.onkeydown = null;
        if (previousActive && typeof previousActive.focus === 'function') {
          previousActive.focus();
        }
        resolve();
      }
      function onTrap(e) {
        if (e.key === 'Escape' || e.key === 'Enter') {
          e.preventDefault();
          close();
          return;
        }
        trapFocus(e, dialogEl);
      }
      okBtn.onclick = close;
      var alertBackdrop = modal.querySelector('.alert-modal-backdrop');
      if (alertBackdrop) alertBackdrop.onclick = close;
      modal.addEventListener('keydown', onTrap);

      modal.setAttribute('aria-hidden', 'false');
      modal.style.display = 'flex';
      okBtn.focus();
    });
  };
})();
