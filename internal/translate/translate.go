// Package translate provides optional machine translation (LibreTranslate-compatible API) with SQLite caching.
package translate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bookstorage/internal/config"
	"bookstorage/internal/database"
)

const (
	frTarget     = "fr"
	httpTimeout  = 20 * time.Second
	maxBodyBytes = 512 * 1024
)

// CachedToFrench returns French text when TranslateURL is set: uses cache or calls the translation API.
// On failure it returns the original text and translated=false (no error, so callers can still serve the page).
func CachedToFrench(db *database.Conn, s *config.Settings, sourceText string) (out string, translated bool, err error) {
	sourceText = strings.TrimSpace(sourceText)
	if s == nil || s.TranslateURL == "" || sourceText == "" {
		return sourceText, false, nil
	}

	sum := sha256.Sum256([]byte(sourceText))
	key := hex.EncodeToString(sum[:])

	var cached string
	qerr := db.QueryRow(
		`SELECT translated_text FROM translation_cache WHERE source_hash = ? AND target_lang = ?`,
		key, frTarget,
	).Scan(&cached)
	if qerr == nil && strings.TrimSpace(cached) != "" {
		return strings.TrimSpace(cached), true, nil
	}
	if qerr != nil && !errors.Is(qerr, sql.ErrNoRows) {
		return sourceText, false, qerr
	}

	translatedText, err := libreTranslate(s, sourceText)
	if err != nil || strings.TrimSpace(translatedText) == "" {
		return sourceText, false, nil
	}
	translatedText = strings.TrimSpace(translatedText)

	ins := `INSERT INTO translation_cache (source_hash, target_lang, translated_text) VALUES (?, ?, ?)
		 ON CONFLICT(source_hash, target_lang) DO UPDATE SET translated_text = excluded.translated_text`
	if db != nil && db.B == database.BackendPostgres {
		ins = `INSERT INTO translation_cache (source_hash, target_lang, translated_text) VALUES (?, ?, ?)
		 ON CONFLICT (source_hash, target_lang) DO UPDATE SET translated_text = EXCLUDED.translated_text`
	}
	if _, err := db.Exec(ins, key, frTarget, translatedText); err != nil {
		return translatedText, true, err
	}
	return translatedText, true, nil
}

type libreRequest struct {
	Q      string `json:"q"`
	Source string `json:"source"`
	Target string `json:"target"`
	Format string `json:"format"`
	APIKey string `json:"api_key,omitempty"`
}

type libreResponse struct {
	TranslatedText string `json:"translatedText"`
	Error          string `json:"error"`
}

func libreTranslate(s *config.Settings, text string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(s.TranslateURL), "/")
	if base == "" {
		return "", fmt.Errorf("empty translate URL")
	}
	u := base + "/translate"

	body, err := json.Marshal(libreRequest{
		Q:      text,
		Source: "auto",
		Target: frTarget,
		Format: "text",
		APIKey: s.TranslateAPIKey,
	})
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	limited := io.LimitReader(resp.Body, maxBodyBytes)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("translate HTTP %d: %s", resp.StatusCode, truncateForLog(raw))
	}

	var out libreResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.Error != "" {
		return "", fmt.Errorf("translate API: %s", out.Error)
	}
	return out.TranslatedText, nil
}

func truncateForLog(b []byte) string {
	const max = 200
	s := string(b)
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
