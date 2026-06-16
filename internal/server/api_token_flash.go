package server

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"sync"
	"time"
)

const apiTokenFlashTTL = 2 * time.Minute

type apiTokenFlashEntry struct {
	userID    int
	token     string
	expiresAt time.Time
}

var (
	apiTokenFlashMu sync.Mutex
	apiTokenFlash   = map[string]apiTokenFlashEntry{}
)

func purgeExpiredAPITokenFlashLocked(now time.Time) {
	for key, entry := range apiTokenFlash {
		if now.After(entry.expiresAt) {
			delete(apiTokenFlash, key)
		}
	}
}

func newAPITokenFlashNonce() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func (a *App) storeAPITokenFlash(userID int, token string) (string, error) {
	nonce, err := newAPITokenFlashNonce()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	apiTokenFlashMu.Lock()
	defer apiTokenFlashMu.Unlock()
	purgeExpiredAPITokenFlashLocked(now)
	apiTokenFlash[nonce] = apiTokenFlashEntry{
		userID:    userID,
		token:     token,
		expiresAt: now.Add(apiTokenFlashTTL),
	}
	return nonce, nil
}

func (a *App) consumeAPITokenFlash(userID int, nonce string) (string, bool) {
	nonce = strings.TrimSpace(nonce)
	if userID <= 0 || nonce == "" {
		return "", false
	}
	now := time.Now().UTC()
	apiTokenFlashMu.Lock()
	defer apiTokenFlashMu.Unlock()
	purgeExpiredAPITokenFlashLocked(now)
	entry, ok := apiTokenFlash[nonce]
	if !ok || entry.userID != userID || now.After(entry.expiresAt) {
		return "", false
	}
	delete(apiTokenFlash, nonce)
	return entry.token, true
}
