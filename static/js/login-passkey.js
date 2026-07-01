(function (global) {
    'use strict';

    var STORAGE_KEY = 'bs_last_username';
    var PASSKEY_HINT_KEY = 'bs_passkey_hint';
    var AUTO_DECLINED_KEY = 'bs_auto_passkey_declined';
    var loginInFlight = false;

    function isMobileUA() {
        return /Android|iPhone|iPad|iPod|Mobile/i.test(navigator.userAgent || '');
    }

    function isStandalonePWA() {
        if (global.navigator && global.navigator.standalone === true) return true;
        try {
            return global.matchMedia('(display-mode: standalone)').matches
                || global.matchMedia('(display-mode: minimal-ui)').matches;
        } catch (e) {
            return false;
        }
    }

    function isAppLaunchContext() {
        return isMobileUA() || isStandalonePWA();
    }

    function isIOS() {
        return /iPhone|iPad|iPod/i.test(navigator.userAgent || '');
    }

    function hasFaceIDHint() {
        if (!isIOS()) return false;
        var ratio = window.screen.width / (window.screen.height || 1);
        return ratio < 0.7 || /iPhone/i.test(navigator.userAgent || '');
    }

    function biometricLabel(i18n) {
        i18n = i18n || {};
        if (isIOS()) {
            return hasFaceIDHint() ? (i18n.faceId || 'Face ID') : (i18n.touchId || 'Touch ID');
        }
        if (/Android/i.test(navigator.userAgent || '')) {
            return i18n.fingerprint || 'Fingerprint';
        }
        return i18n.generic || 'Passkey';
    }

    function biometricIcon() {
        if (isIOS()) return hasFaceIDHint() ? '👤' : '👆';
        if (/Android/i.test(navigator.userAgent || '')) return '👆';
        return '🔐';
    }

    function webAuthnSupported() {
        return !!(global.PublicKeyCredential && global.WebAuthnClient);
    }

    function getLastUsername() {
        try {
            return (localStorage.getItem(STORAGE_KEY) || '').trim();
        } catch (e) {
            return '';
        }
    }

    function saveLastUsername(username) {
        try {
            if (username) localStorage.setItem(STORAGE_KEY, username);
        } catch (e) { /* ignore */ }
    }

    function hasPasskeyHint() {
        try {
            return localStorage.getItem(PASSKEY_HINT_KEY) === '1';
        } catch (e) {
            return false;
        }
    }

    function markPasskeyHint() {
        try {
            localStorage.setItem(PASSKEY_HINT_KEY, '1');
        } catch (e) { /* ignore */ }
    }

    function shouldAutoPromptPasskey() {
        if (!webAuthnSupported() || !isAppLaunchContext()) return false;
        if (!hasPasskeyHint() && !isStandalonePWA()) return false;
        try {
            if (sessionStorage.getItem(AUTO_DECLINED_KEY) === '1') return false;
        } catch (e) { /* ignore */ }
        try {
            var params = new URLSearchParams(global.location.search || '');
            if (params.get('webauthn_error')) return false;
        } catch (e2) { /* ignore */ }
        return true;
    }

    function setPasskeyPending(active) {
        var card = document.querySelector('.auth-card');
        if (card) card.classList.toggle('auth-card--passkey-pending', !!active);
    }

    function finishURL(next) {
        var url = '/auth/webauthn/login/finish';
        if (next) url += '?next=' + encodeURIComponent(next);
        return url;
    }

    function redirectLoginError(next, code) {
        var url = '/login?webauthn_error=' + encodeURIComponent(code || 'failed');
        if (next) url += '&next=' + encodeURIComponent(next);
        global.location.href = url;
    }

    function isUserCancel(err) {
        if (!err) return false;
        var name = err.name || '';
        return name === 'NotAllowedError' || name === 'AbortError';
    }

    async function runPasskeyLogin(opts) {
        opts = opts || {};
        var username = (opts.username || '').trim();
        var discoverable = !!opts.discoverable;
        var next = opts.next || '';

        var body = { discoverable: discoverable };
        if (username) body.username = username;

        var begin = await fetch('/auth/webauthn/login/begin', {
            method: 'POST',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body)
        });
        if (!begin.ok) {
            var beginErr = await begin.json().catch(function () { return {}; });
            throw new Error(beginErr.error || 'begin_failed');
        }

        var request = WebAuthnClient.decodeRequestOptions(await begin.json());
        if (!request) throw new Error('invalid_options');

        var cred;
        try {
            cred = await navigator.credentials.get(request);
        } catch (e) {
            if (isUserCancel(e)) throw e;
            throw new Error('assertion_failed');
        }
        if (!cred) throw new Error('assertion_failed');

        var finish = await fetch(finishURL(next), {
            method: 'POST',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(WebAuthnClient.credentialToJSON(cred))
        });
        var data = await finish.json().catch(function () { return {}; });
        if (data.ok && data.redirect) {
            if (username) saveLastUsername(username);
            return data.redirect;
        }
        throw new Error(data.error || 'finish_failed');
    }

    async function loginWithPasskey(username, next) {
        username = (username || '').trim();
        var strategies = [];
        if (isMobileUA() || !username) {
            strategies.push({ discoverable: true, username: '' });
        }
        if (username) {
            strategies.push({ discoverable: false, username: username });
        }
        if (!strategies.length) {
            throw new Error('username_required');
        }

        var lastErr = null;
        for (var i = 0; i < strategies.length; i++) {
            try {
                return await runPasskeyLogin({
                    discoverable: strategies[i].discoverable,
                    username: strategies[i].username,
                    next: next
                });
            } catch (e) {
                if (isUserCancel(e)) throw e;
                lastErr = e;
            }
        }
        throw lastErr || new Error('failed');
    }

    async function attemptPasskeyLogin(username, next, opts) {
        opts = opts || {};
        if (loginInFlight) return false;
        loginInFlight = true;
        if (opts.showPending) setPasskeyPending(true);
        try {
            var redirect = await loginWithPasskey(username, next);
            markPasskeyHint();
            global.location.href = redirect;
            return true;
        } catch (e) {
            if (isUserCancel(e)) {
                if (opts.markDeclined) {
                    try { sessionStorage.setItem(AUTO_DECLINED_KEY, '1'); } catch (err) { /* ignore */ }
                }
                return false;
            }
            if (!opts.silent) {
                var code = (e && e.message) ? e.message : 'failed';
                redirectLoginError(next, code);
            }
            return false;
        } finally {
            loginInFlight = false;
            if (opts.showPending) setPasskeyPending(false);
        }
    }

    async function tryAutoPasskeyOnLaunch(usernameInput, next) {
        if (!shouldAutoPromptPasskey()) return;
        var username = (usernameInput && usernameInput.value || getLastUsername() || '').trim();
        await new Promise(function (resolve) { global.setTimeout(resolve, 150); });
        await attemptPasskeyLogin(username, next, {
            silent: true,
            markDeclined: true,
            showPending: true
        });
    }

    function initLoginPasskey(config) {
        config = config || {};
        var i18n = config.i18n || {};
        var next = config.next || '';
        var usernameInput = document.getElementById('username');
        var btn = document.getElementById('passkey-login-btn');
        var labelEl = document.getElementById('passkey-login-label');
        var iconEl = document.getElementById('passkey-login-icon');
        var block = document.getElementById('passkey-login-block');

        if (!webAuthnSupported()) {
            if (block) block.hidden = true;
            return;
        }

        var label = biometricLabel(i18n);
        if (labelEl) labelEl.textContent = label;
        if (iconEl) iconEl.textContent = biometricIcon();
        if (block) block.classList.toggle('auth-biometric-block--mobile', isMobileUA());

        var saved = getLastUsername();
        if (usernameInput && saved && !usernameInput.value) {
            usernameInput.value = saved;
        }
        if (usernameInput) {
            usernameInput.setAttribute('autocomplete', 'username webauthn');
        }

        var loginForm = usernameInput && usernameInput.closest('form');
        if (loginForm) {
            loginForm.addEventListener('submit', function () {
                saveLastUsername((usernameInput.value || '').trim());
            });
        }

        btn && btn.addEventListener('click', async function () {
            if (btn.disabled || loginInFlight) return;
            btn.disabled = true;
            try {
                var username = (usernameInput && usernameInput.value || '').trim();
                if (!username && !isMobileUA()) {
                    alert(i18n.usernameRequired || 'Enter your username.');
                    return;
                }
                await attemptPasskeyLogin(username, next, { silent: false });
            } finally {
                btn.disabled = false;
            }
        });

        if (config.autoPrompt !== false) {
            tryAutoPasskeyOnLaunch(usernameInput, next);
        }
    }

    global.LoginPasskey = {
        init: initLoginPasskey,
        biometricLabel: biometricLabel,
        isMobile: isMobileUA,
        markPasskeyHint: markPasskeyHint
    };
})(typeof window !== 'undefined' ? window : globalThis);
