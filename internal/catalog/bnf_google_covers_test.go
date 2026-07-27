package catalog

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeISBNDigits(t *testing.T) {
	if got := NormalizeISBNDigits("978-2-205-07000-0"); got != "9782205070000" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeISBNDigits(" 0-306-40615-X "); got != "030640615X" {
		t.Fatalf("got %q", got)
	}
}

func TestLookupBnFCoverByISBN(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Query().Get("EAN") == "9782205070000" {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte{0xff, 0xd8, 0xff})
			return
		}
		http.Error(w, "no cover", http.StatusInternalServerError)
	}))
	defer srv.Close()

	origBase, origClient := bnfCoverBaseURL, bnfHTTPClient
	bnfCoverBaseURL = srv.URL
	bnfHTTPClient = srv.Client()
	defer func() {
		bnfCoverBaseURL = origBase
		bnfHTTPClient = origClient
	}()

	u, err := LookupBnFCoverByISBN("978-2-205-07000-0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "EAN=9782205070000") || !strings.Contains(u, "couverture=1") {
		t.Fatalf("url=%q hits=%d", u, hits)
	}
}

func TestLookupGoogleBooksCoverByISBN(t *testing.T) {
	t.Setenv("BOOKSTORAGE_GOOGLE_BOOKS_API_KEY", "test-key")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "isbn%3A9782205070000") && !strings.Contains(r.URL.RawQuery, "isbn:9782205070000") {
			t.Fatalf("query=%q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"volumeInfo":{"title":"Astérix","imageLinks":{"thumbnail":"http://books.google.com/cover?zoom=1"}}}]}`))
	}))
	defer srv.Close()

	origURL, origClient := googleBooksSearchURL, googleBooksHTTPClient
	googleBooksSearchURL = srv.URL
	googleBooksHTTPClient = srv.Client()
	defer func() {
		googleBooksSearchURL = origURL
		googleBooksHTTPClient = origClient
	}()

	u, err := LookupGoogleBooksCoverByISBN("9782205070000")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "https://") || !strings.Contains(u, "zoom=2") {
		t.Fatalf("url=%q", u)
	}
}

func TestLookupGoogleBooksCover_SoftSkipsRateLimit(t *testing.T) {
	t.Setenv("BOOKSTORAGE_GOOGLE_BOOKS_API_KEY", "test-key")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "quota", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	origURL, origClient := googleBooksSearchURL, googleBooksHTTPClient
	googleBooksSearchURL = srv.URL
	googleBooksHTTPClient = srv.Client()
	defer func() {
		googleBooksSearchURL = origURL
		googleBooksHTTPClient = origClient
	}()

	u, err := LookupGoogleBooksCoverByISBN("9782205070000")
	if err != nil {
		t.Fatalf("expected soft skip, got err %v", err)
	}
	if u != "" {
		t.Fatalf("url=%q", u)
	}
}

func TestLookupGoogleBooksCover_SkippedWithoutAPIKey(t *testing.T) {
	t.Setenv("BOOKSTORAGE_GOOGLE_BOOKS_API_KEY", "")
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()
	origURL, origClient := googleBooksSearchURL, googleBooksHTTPClient
	googleBooksSearchURL = srv.URL
	googleBooksHTTPClient = srv.Client()
	defer func() {
		googleBooksSearchURL = origURL
		googleBooksHTTPClient = origClient
	}()

	u, err := LookupGoogleBooksCoverByISBN("9782205070000")
	if err != nil || u != "" || called {
		t.Fatalf("u=%q err=%v called=%v", u, err, called)
	}
}

func TestLookupBnFCover_SoftSkipsRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	origBase, origClient := bnfCoverBaseURL, bnfHTTPClient
	bnfCoverBaseURL = srv.URL
	bnfHTTPClient = srv.Client()
	defer func() {
		bnfCoverBaseURL = origBase
		bnfHTTPClient = origClient
	}()

	u, err := LookupBnFCoverByISBN("9782205070000")
	if err != nil {
		t.Fatalf("expected soft skip, got %v", err)
	}
	if u != "" {
		t.Fatalf("url=%q", u)
	}
}
