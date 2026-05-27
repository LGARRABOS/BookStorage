document.addEventListener('DOMContentLoaded', function() {
    document.addEventListener('click', function(e) {
        const del = e.target.closest('.js-confirm-delete');
        if (del) {
            e.preventDefault();
            const form = del.closest('form');
            if (!form) return;
            const msg = del.dataset.confirm || document.body.dataset.confirmPrompt || 'Confirm?';
            showConfirm(msg).then(function(ok) { if (ok) form.submit(); });
        }
    });
});
