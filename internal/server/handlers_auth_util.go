package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
)

const minPasswordLen = 8

// passwordHashNeedsUpgrade reports whether a stored hash should be re-hashed to bcrypt on next login.
func passwordHashNeedsUpgrade(stored string) bool {
	s := strings.TrimSpace(stored)
	if s == "" {
		return false
	}
	return !strings.HasPrefix(s, "$2a$") && !strings.HasPrefix(s, "$2b$") && !strings.HasPrefix(s, "$2y$")
}

// hashPassword hashes a password using bcrypt.
func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// verifyPassword checks a password against a hash.
// Supports bcrypt ($2a$, $2b$, $2y$) and Werkzeug pbkdf2:sha256:iterations$salt$hash.
func verifyPassword(storedHash, password string) bool {
	if strings.HasPrefix(storedHash, "$2a$") || strings.HasPrefix(storedHash, "$2b$") || strings.HasPrefix(storedHash, "$2y$") {
		err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password))
		return err == nil
	}
	if strings.HasPrefix(storedHash, "pbkdf2:") {
		return verifyWerkzeugHash(storedHash, password)
	}
	return false
}

// verifyWerkzeugHash checks a password against a Werkzeug hash.
// Format: pbkdf2:sha256:iterations$salt$hash
func verifyWerkzeugHash(storedHash, password string) bool {
	// Split method$salt$hash
	parts := strings.SplitN(storedHash, "$", 3)
	if len(parts) != 3 {
		return false
	}

	method := parts[0]  // pbkdf2:sha256:iterations
	salt := parts[1]    // salt en clair
	hashHex := parts[2] // hex-encoded hash

	// Extract method parameters
	methodParts := strings.Split(method, ":")
	if len(methodParts) < 3 || methodParts[0] != "pbkdf2" || methodParts[1] != "sha256" {
		return false // Unsupported format
	}

	iterations := 260000 // Werkzeug default
	if len(methodParts) >= 3 {
		if n, err := strconv.Atoi(methodParts[2]); err == nil {
			iterations = n
		}
	}

	// Decode expected hash
	expectedHash, err := hex.DecodeString(hashHex)
	if err != nil {
		return false
	}

	// Calculer le hash PBKDF2
	computed := pbkdf2.Key([]byte(password), []byte(salt), iterations, len(expectedHash), sha256.New)

	// Constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare(computed, expectedHash) == 1
}

const maxPostLoginRedirectLen = 1024

// safePostLoginRedirect returns a same-origin path+query safe to use after login, or "".
func safePostLoginRedirect(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > maxPostLoginRedirectLen {
		return ""
	}
	if !strings.HasPrefix(s, "/") {
		return ""
	}
	if strings.HasPrefix(s, "//") {
		return ""
	}
	if strings.Contains(s, "://") {
		return ""
	}
	if strings.ContainsAny(s, "\r\n\x00") {
		return ""
	}
	if strings.Contains(s, `\`) {
		return ""
	}
	if strings.HasPrefix(s, "/login") || strings.HasPrefix(s, "/register") {
		return ""
	}
	return s
}

// safeLanguageRedirect returns a path+query from Referer only if same host as r; otherwise fallback.
// Prevents open redirects via Referer after POST/GET /lang/{lang}.
func safeLanguageRedirect(r *http.Request, fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if fallback == "" || !strings.HasPrefix(fallback, "/") || strings.HasPrefix(fallback, "//") {
		fallback = "/dashboard"
	}
	ref := strings.TrimSpace(r.Header.Get("Referer"))
	if ref == "" {
		return fallback
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" {
		return fallback
	}
	if !strings.EqualFold(u.Host, r.Host) {
		return fallback
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return fallback
	}
	if strings.ContainsAny(path, "\r\n\x00") {
		return fallback
	}
	if u.RawQuery != "" {
		return path + "?" + u.RawQuery
	}
	return path
}

func loginRedirectURL(r *http.Request) string {
	if r == nil || r.URL == nil {
		return "/login"
	}
	next := r.URL.Path
	if r.URL.RawQuery != "" {
		next += "?" + r.URL.RawQuery
	}
	next = safePostLoginRedirect(next)
	if next == "" {
		return "/login"
	}
	return "/login?next=" + url.QueryEscape(next)
}
