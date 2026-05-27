package server

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

type webauthnChallengeEntry struct {
	Data      webauthn.SessionData
	UserID    int
	ExpiresAt time.Time
}

type webauthnChallengeStore struct {
	mu      sync.Mutex
	entries map[string]webauthnChallengeEntry
}

func newWebAuthnChallengeStore() *webauthnChallengeStore {
	return &webauthnChallengeStore{entries: map[string]webauthnChallengeEntry{}}
}

func (s *webauthnChallengeStore) put(data *webauthn.SessionData, userID int) (key string, err error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	key = base64.RawURLEncoding.EncodeToString(b[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeLocked()
	s.entries[key] = webauthnChallengeEntry{
		Data:      *data,
		UserID:    userID,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	return key, nil
}

func (s *webauthnChallengeStore) take(key string) (webauthn.SessionData, int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeLocked()
	ent, ok := s.entries[key]
	if !ok || time.Now().After(ent.ExpiresAt) {
		delete(s.entries, key)
		return webauthn.SessionData{}, 0, false
	}
	delete(s.entries, key)
	return ent.Data, ent.UserID, true
}

func (s *webauthnChallengeStore) purgeLocked() {
	now := time.Now()
	for k, v := range s.entries {
		if now.After(v.ExpiresAt) {
			delete(s.entries, k)
		}
	}
}
