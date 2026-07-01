(function (global) {
    'use strict';

    function init(opts) {
        opts = opts || {};
        var btn = document.getElementById('passkey-register-btn');
        if (!btn || !global.WebAuthnClient) return;

        btn.addEventListener('click', async function () {
            if (btn.disabled) return;
            btn.disabled = true;
            var successUrl = opts.successUrl || '/profile?webauthn_registered=1';
            var errorBase = opts.errorBase || '/profile?webauthn_error=';
            try {
                var begin = await fetch('/auth/webauthn/register/begin', { method: 'POST', credentials: 'same-origin' });
                if (!begin.ok) { global.location.href = errorBase + 'server'; return; }
                var creation = WebAuthnClient.decodeRequestOptions(await begin.json());
                if (!creation) { global.location.href = errorBase + 'server'; return; }
                var cred = await navigator.credentials.create(creation);
                var suggested = WebAuthnClient.suggestPasskeyLabel(cred);
                var entered = global.prompt(opts.namePrompt || 'Name for this Passkey', suggested);
                if (entered === null) return;
                var name = (entered.trim() || suggested || 'Passkey').trim();
                var finish = await fetch('/auth/webauthn/register/finish?name=' + encodeURIComponent(name), {
                    method: 'POST',
                    credentials: 'same-origin',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(WebAuthnClient.credentialToJSON(cred))
                });
                if (finish.ok) { global.location.href = successUrl; return; }
                var errBody = await finish.json().catch(function () { return {}; });
                global.location.href = errorBase + encodeURIComponent(errBody.error || 'server');
            } catch (e) {
                global.location.href = errorBase + 'server';
            } finally {
                btn.disabled = false;
            }
        });
    }

    global.PasskeyRegister = { init: init };
})(typeof window !== 'undefined' ? window : globalThis);
