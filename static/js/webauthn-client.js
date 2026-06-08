(function (global) {
    'use strict';

    function bufferToBase64url(buffer) {
        const bytes = new Uint8Array(buffer);
        let binary = '';
        for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
        return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
    }

    function decodeBase64url(value) {
        return Uint8Array.from(atob(String(value).replace(/-/g, '+').replace(/_/g, '/')), function (c) {
            return c.charCodeAt(0);
        });
    }

    function decodeRequestOptions(body) {
        if (!body || !body.publicKey) return null;
        const options = body.publicKey;
        options.challenge = decodeBase64url(options.challenge);
        if (options.user && options.user.id) options.user.id = decodeBase64url(options.user.id);
        if (options.excludeCredentials) {
            options.excludeCredentials = options.excludeCredentials.map(function (c) {
                return Object.assign({}, c, { id: decodeBase64url(c.id) });
            });
        }
        if (options.allowCredentials) {
            options.allowCredentials = options.allowCredentials.map(function (c) {
                return Object.assign({}, c, { id: decodeBase64url(c.id) });
            });
        }
        return body;
    }

    function credentialToJSON(cred) {
        if (!cred) throw new Error('missing credential');
        if (typeof cred.toJSON === 'function') return cred.toJSON();
        const response = cred.response;
        if (!response || !response.clientDataJSON) throw new Error('unsupported credential');
        const out = {
            id: cred.id,
            rawId: bufferToBase64url(cred.rawId),
            type: cred.type,
            clientExtensionResults: cred.getClientExtensionResults ? cred.getClientExtensionResults() : {},
            response: {
                clientDataJSON: bufferToBase64url(response.clientDataJSON)
            }
        };
        if (response.attestationObject) {
            out.response.attestationObject = bufferToBase64url(response.attestationObject);
        }
        if (response.authenticatorData) {
            out.response.authenticatorData = bufferToBase64url(response.authenticatorData);
        }
        if (response.signature) {
            out.response.signature = bufferToBase64url(response.signature);
        }
        if (response.userHandle) {
            out.response.userHandle = bufferToBase64url(response.userHandle);
        }
        if (typeof response.getTransports === 'function') {
            const transports = response.getTransports();
            if (transports && transports.length) out.response.transports = transports;
        }
        if (cred.authenticatorAttachment) out.authenticatorAttachment = cred.authenticatorAttachment;
        return out;
    }

    function suggestPasskeyLabel(cred) {
        const attachment = cred && cred.authenticatorAttachment;
        const ua = navigator.userAgent || '';
        if (/Macintosh|Mac OS X/i.test(ua) && attachment === 'platform') return 'Mac (Touch ID)';
        if (/Windows/i.test(ua) && attachment === 'platform') return 'Windows (code PIN)';
        if (/iPhone|iPad|iPod/i.test(ua) && attachment === 'platform') return 'iPhone / iPad';
        if (/Android/i.test(ua) && attachment === 'platform') return 'Android';
        if (attachment === 'cross-platform') return 'Clé de sécurité';
        if (attachment === 'platform') return 'Cet appareil';
        return 'Passkey';
    }

    global.WebAuthnClient = {
        decodeRequestOptions: decodeRequestOptions,
        credentialToJSON: credentialToJSON,
        suggestPasskeyLabel: suggestPasskeyLabel
    };
})(typeof window !== 'undefined' ? window : globalThis);
