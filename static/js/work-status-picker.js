(function () {
    'use strict';

    var activePicker = null;
    var menuEl = null;
    var busy = false;

    function getCard(picker) {
        return picker.closest('.work-card') || picker.closest('.work-mobile-card');
    }

    function closeMenu() {
        if (!menuEl) return;
        menuEl.hidden = true;
        if (activePicker) {
            activePicker.setAttribute('aria-expanded', 'false');
            activePicker = null;
        }
    }

    function positionMenu(picker) {
        if (!menuEl) return;
        var rect = picker.getBoundingClientRect();
        menuEl.style.position = 'fixed';
        menuEl.style.top = (rect.bottom + 4) + 'px';
        menuEl.style.left = rect.left + 'px';
        menuEl.style.zIndex = '9999';
        menuEl.hidden = false;
        requestAnimationFrame(function () {
            var mRect = menuEl.getBoundingClientRect();
            if (mRect.right > window.innerWidth - 8) {
                menuEl.style.left = Math.max(8, window.innerWidth - mRect.width - 8) + 'px';
            }
            if (mRect.bottom > window.innerHeight - 8) {
                menuEl.style.top = Math.max(8, rect.top - mRect.height - 4) + 'px';
            }
        });
    }

    function markActiveItem(status) {
        if (!menuEl) return;
        menuEl.querySelectorAll('.work-status-menu-item').forEach(function (item) {
            var isActive = item.getAttribute('data-status') === status;
            item.classList.toggle('is-active', isActive);
            item.setAttribute('aria-checked', isActive ? 'true' : 'false');
        });
    }

    function applyStatusToPicker(picker, status, label) {
        picker.setAttribute('data-status', status);
        picker.textContent = label;
        var card = getCard(picker);
        if (card) card.setAttribute('data-status', status);
        document.dispatchEvent(new CustomEvent('workstatuschanged', {
            detail: { workId: picker.getAttribute('data-work-id'), status: status }
        }));
    }

    function patchWorkStatus(workId, status) {
        return fetch('/api/works/' + workId, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({ status: status })
        }).then(function (resp) {
            if (resp.status === 401) {
                window.location.href = '/login?expired=1';
                return false;
            }
            return resp.ok;
        });
    }

    function openMenu(picker) {
        if (busy) return;
        if (activePicker === picker && menuEl && !menuEl.hidden) {
            closeMenu();
            return;
        }
        closeMenu();
        activePicker = picker;
        picker.setAttribute('aria-expanded', 'true');
        markActiveItem(picker.getAttribute('data-status') || '');
        positionMenu(picker);
    }

    function initMenu() {
        var tpl = document.getElementById('work-status-menu-tpl');
        if (!tpl) return;
        menuEl = tpl.content.firstElementChild.cloneNode(true);
        document.body.appendChild(menuEl);
        menuEl.addEventListener('click', function (e) {
            var item = e.target.closest('.work-status-menu-item');
            if (!item || !activePicker || busy) return;
            e.preventDefault();
            e.stopPropagation();
            var status = item.getAttribute('data-status');
            var label = item.getAttribute('data-label') || item.textContent;
            var workId = activePicker.getAttribute('data-work-id');
            if (!workId || !status) return;
            var current = activePicker.getAttribute('data-status') || '';
            if (status === current) {
                closeMenu();
                return;
            }
            var picker = activePicker;
            closeMenu();
            busy = true;
            picker.setAttribute('aria-busy', 'true');
            patchWorkStatus(workId, status).then(function (ok) {
                if (ok) {
                    applyStatusToPicker(picker, status, label);
                } else {
                    picker.classList.add('work-status-picker--error');
                    setTimeout(function () { picker.classList.remove('work-status-picker--error'); }, 1500);
                }
            }).catch(function () {
                picker.classList.add('work-status-picker--error');
                setTimeout(function () { picker.classList.remove('work-status-picker--error'); }, 1500);
            }).finally(function () {
                picker.removeAttribute('aria-busy');
                busy = false;
            });
        });
        document.addEventListener('click', function (e) {
            if (menuEl && !menuEl.hidden && !e.target.closest('.work-status-menu') && !e.target.closest('.work-status-picker')) {
                closeMenu();
            }
        });
        document.addEventListener('keydown', function (e) {
            if (e.key === 'Escape') closeMenu();
        });
        window.addEventListener('scroll', closeMenu, true);
        window.addEventListener('resize', closeMenu);
    }

    function initPickers() {
        document.querySelectorAll('.work-status-picker').forEach(function (picker) {
            if (picker.dataset.statusPickerInit) return;
            picker.dataset.statusPickerInit = '1';
            picker.addEventListener('click', function (e) {
                e.preventDefault();
                e.stopPropagation();
                openMenu(picker);
            });
        });
    }

    document.addEventListener('DOMContentLoaded', function () {
        initMenu();
        initPickers();
    });

    window.initWorkStatusPickers = initPickers;
})();
