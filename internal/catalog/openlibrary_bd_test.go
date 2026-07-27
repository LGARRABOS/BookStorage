package catalog

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSearchOpenLibraryBD(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"numFound": 1,
			"docs": [{
				"key": "/works/OL123W",
				"title": "Astérix le Gaulois",
				"author_name": ["René Goscinny", "Albert Uderzo"],
				"cover_i": 42,
				"first_publish_year": 1961,
				"subject": ["Bandes dessinées", "Humour"]
			}]
		}`))
	}))
	defer srv.Close()

	origBase, origClient := olSearchBase, olHTTPClient
	olSearchBase = srv.URL
	olHTTPClient = srv.Client()
	olLastCall = time.Time{}
	defer func() {
		olSearchBase = origBase
		olHTTPClient = origClient
	}()

	got, err := SearchOpenLibraryBD("Astérix", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != "/works/OL123W" || got[0].Title != "Astérix le Gaulois" {
		t.Fatalf("unexpected %+v", got[0])
	}
	if !strings.Contains(got[0].ImageURL, "/b/id/42-L.jpg") {
		t.Fatalf("cover %q", got[0].ImageURL)
	}
	if got[0].BdType != "Album" {
		t.Fatalf("bd_type %q", got[0].BdType)
	}
	if got[0].FirstPublishYear != 1961 {
		t.Fatalf("year %d", got[0].FirstPublishYear)
	}
}

func TestBrowseOpenLibraryBD(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if !strings.Contains(strings.ToLower(q), "bande") {
			t.Fatalf("expected bande subject query, got %q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"numFound":0,"docs":[]}`))
	}))
	defer srv.Close()

	origBase, origClient := olSearchBase, olHTTPClient
	olSearchBase = srv.URL
	olHTTPClient = srv.Client()
	olLastCall = time.Time{}
	defer func() {
		olSearchBase = origBase
		olHTTPClient = origClient
	}()

	got, err := BrowseOpenLibraryBD(1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("len=%d", len(got))
	}
}

func TestMapOpenLibraryBdType(t *testing.T) {
	if got := mapOpenLibraryBdType([]string{"Comics", "One-shot"}); got != "One-shot" {
		t.Fatalf("got %q", got)
	}
	if got := mapOpenLibraryBdType([]string{"Intégrale Tintin"}); got != "Intégrale" {
		t.Fatalf("got %q", got)
	}
}

func TestSearchOpenLibraryBDCover_NoLanguageFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("language") != "" {
			t.Fatalf("cover search should omit language, got %q", r.URL.Query().Get("language"))
		}
		q := r.URL.Query().Get("q")
		if strings.Contains(strings.ToLower(q), "bande dessinée") {
			t.Fatalf("cover search should be plain title, got %q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"numFound": 1,
			"docs": [{
				"key": "/works/OL9W",
				"title": "Aigles de Rome",
				"cover_i": 7
			}]
		}`))
	}))
	defer srv.Close()

	origBase, origClient := olSearchBase, olHTTPClient
	olSearchBase = srv.URL
	olHTTPClient = srv.Client()
	olLastCall = time.Time{}
	defer func() {
		olSearchBase = origBase
		olHTTPClient = origClient
	}()

	got, err := SearchOpenLibraryBDCover("Aigles de Rome (Les) — Livre VII", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ImageURL == "" {
		t.Fatalf("got %+v", got)
	}
}
