package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	googleBooksTimeout = 8 * time.Second
	googleBooksMaxBody = 2 << 20
)

var (
	googleBooksSearchURL  = "https://www.googleapis.com/books/v1/volumes"
	googleBooksHTTPClient = &http.Client{Timeout: googleBooksTimeout}
)

// googleBooksAPIKey is optional; BOOKSTORAGE_GOOGLE_BOOKS_API_KEY raises Google Books quota.
func googleBooksAPIKey() string {
	return strings.TrimSpace(os.Getenv("BOOKSTORAGE_GOOGLE_BOOKS_API_KEY"))
}

type googleBooksVolumeList struct {
	Items []struct {
		VolumeInfo struct {
			Title      string `json:"title"`
			ImageLinks struct {
				Thumbnail string `json:"thumbnail"`
				Small     string `json:"small"`
				Medium    string `json:"medium"`
				Large     string `json:"large"`
			} `json:"imageLinks"`
		} `json:"volumeInfo"`
	} `json:"items"`
}

// LookupGoogleBooksCoverByISBN returns the best available cover URL for an ISBN.
func LookupGoogleBooksCoverByISBN(rawISBN string) (string, error) {
	digits := NormalizeISBNDigits(rawISBN)
	if digits == "" {
		return "", nil
	}
	return googleBooksSearch("isbn:"+digits, 1)
}

// LookupGoogleBooksCoverByTitle returns a cover URL for a free-text title search.
func LookupGoogleBooksCoverByTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", nil
	}
	return googleBooksSearch(`intitle:"`+title+`"`, 3)
}

func googleBooksSearch(q string, maxResults int) (string, error) {
	if maxResults <= 0 {
		maxResults = 1
	}
	params := url.Values{}
	params.Set("q", q)
	params.Set("maxResults", fmt.Sprintf("%d", maxResults))
	params.Set("printType", "books")
	if key := googleBooksAPIKey(); key != "" {
		params.Set("key", key)
	}

	ctx, cancel := context.WithTimeout(context.Background(), googleBooksTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleBooksSearchURL+"?"+params.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", openLibraryUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := googleBooksHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", ErrOpenLibraryRateLimit
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("google books: http %d", resp.StatusCode)
	}
	var parsed googleBooksVolumeList
	if err := json.NewDecoder(io.LimitReader(resp.Body, googleBooksMaxBody)).Decode(&parsed); err != nil {
		return "", err
	}
	for _, item := range parsed.Items {
		links := item.VolumeInfo.ImageLinks
		for _, u := range []string{links.Large, links.Medium, links.Small, links.Thumbnail} {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			u = strings.Replace(u, "http://", "https://", 1)
			// Prefer larger edge size when Google serves zoomed thumbnails.
			u = strings.Replace(u, "zoom=1", "zoom=2", 1)
			return u, nil
		}
	}
	return "", nil
}
