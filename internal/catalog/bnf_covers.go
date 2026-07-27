package catalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const bnfCoverTimeout = 8 * time.Second

var (
	bnfCoverBaseURL = "https://openapi.bnf.fr/couverture/image/image/recupererImage"
	bnfHTTPClient   = &http.Client{Timeout: bnfCoverTimeout}
)

// NormalizeISBNDigits keeps digits and trailing X (ISBN-10 check digit).
func NormalizeISBNDigits(raw string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		if r == 'x' || r == 'X' {
			b.WriteByte('X')
		}
	}
	return b.String()
}

// LookupBnFCoverByISBN returns a BnF cover image URL when the notice has a front cover.
// Tries EAN first (13-digit), then ISBN digits once. BnF returns HTTP 500 when no cover exists.
func LookupBnFCoverByISBN(rawISBN string) (string, error) {
	digits := NormalizeISBNDigits(rawISBN)
	if digits == "" {
		return "", nil
	}
	// Prefer a single probe: EAN for ISBN-13, else ISBN.
	if len(digits) == 13 {
		return probeBnFCover("EAN", digits)
	}
	return probeBnFCover("ISBN", digits)
}

func probeBnFCover(param, value string) (string, error) {
	q := url.Values{}
	q.Set(param, value)
	q.Set("couverture", "1")
	q.Set("taille", "originale")
	q.Set("largeur", "500")
	q.Set("hauteur", "800")
	coverURL := bnfCoverBaseURL + "?" + q.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), bnfCoverTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, coverURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", openLibraryUserAgent)
	req.Header.Set("Accept", "image/*,*/*")

	resp, err := bnfHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	// Only sniff a tiny prefix so cover enrichment does not stream large images.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))

	if resp.StatusCode == http.StatusTooManyRequests {
		// Soft-skip: do not pause the whole BD cover job (BnF is optional in the chain).
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		// BnF documents HTTP 500 when the notice has no cover image.
		return "", nil
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if ct != "" && !strings.HasPrefix(ct, "image/") && !strings.Contains(ct, "octet-stream") {
		return "", nil
	}
	return coverURL, nil
}

// OpenLibraryCoverByISBN returns covers.openlibrary.org URL when the ISBN has a cover (?default=false → 404 if missing).
func OpenLibraryCoverByISBN(rawISBN string) (string, error) {
	digits := NormalizeISBNDigits(rawISBN)
	if len(digits) < 10 {
		return "", nil
	}
	coverURL := fmt.Sprintf("https://covers.openlibrary.org/b/isbn/%s-L.jpg?default=false", digits)
	ctx, cancel := context.WithTimeout(context.Background(), openLibraryDefaultTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, coverURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", openLibraryUserAgent)
	resp, err := olHTTPClient.Do(req)
	if err != nil {
		// Some CDNs reject HEAD; fall back to GET with tiny read.
		req2, err2 := http.NewRequestWithContext(ctx, http.MethodGet, coverURL, nil)
		if err2 != nil {
			return "", err
		}
		req2.Header.Set("User-Agent", openLibraryUserAgent)
		resp, err = olHTTPClient.Do(req2)
		if err != nil {
			return "", err
		}
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode == http.StatusTooManyRequests {
		// Soft-skip cover CDN throttling; search.json rate limits remain hard.
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil
	}
	return fmt.Sprintf("https://covers.openlibrary.org/b/isbn/%s-L.jpg", digits), nil
}
