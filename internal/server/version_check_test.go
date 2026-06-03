package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseSemver(t *testing.T) {
	tests := []struct {
		raw                 string
		major, minor, patch int
		ok                  bool
	}{
		{"6.1.1", 6, 1, 1, true},
		{"v6.1.1", 6, 1, 1, true},
		{"V6.1.1", 6, 1, 1, true},
		{"6.1", 6, 1, 0, true},
		{"6", 6, 0, 0, true},
		{"6.1.1-beta", 6, 1, 1, true},
		{"dev", 0, 0, 0, false},
		{"", 0, 0, 0, false},
		{"abc", 0, 0, 0, false},
	}
	for _, tc := range tests {
		major, minor, patch, ok := parseSemver(tc.raw)
		if ok != tc.ok || major != tc.major || minor != tc.minor || patch != tc.patch {
			t.Errorf("parseSemver(%q) = (%d,%d,%d,%v), want (%d,%d,%d,%v)",
				tc.raw, major, minor, patch, ok, tc.major, tc.minor, tc.patch, tc.ok)
		}
	}
}

func TestSemverCompare(t *testing.T) {
	if semverCompare("6.1.0", "6.1.1") >= 0 {
		t.Fatal("6.1.0 should be less than 6.1.1")
	}
	if semverCompare("v6.2.0", "6.1.9") <= 0 {
		t.Fatal("6.2.0 should be greater than 6.1.9")
	}
	if semverCompare("6.1.1", "v6.1.1") != 0 {
		t.Fatal("6.1.1 should equal v6.1.1")
	}
}

func TestCheckUpdateAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName: "v6.9.0",
			HTMLURL: "https://github.com/LGARRABOS/BookStorage/releases/tag/v6.9.0",
		})
	}))
	defer srv.Close()

	origURL := githubReleasesLatestURL
	githubReleasesLatestURL = srv.URL
	t.Cleanup(func() { githubReleasesLatestURL = origURL })

	app := &App{Version: "6.1.1"}
	available, latest, url := app.checkUpdateAvailable()
	if !available {
		t.Fatal("expected update available")
	}
	if latest != "v6.9.0" {
		t.Fatalf("latest=%q", latest)
	}
	if url == "" {
		t.Fatal("expected release URL")
	}

	app.Version = "6.9.0"
	available, _, _ = app.checkUpdateAvailable()
	if available {
		t.Fatal("expected no update when current equals latest")
	}

	app.Version = "dev"
	available, _, _ = app.checkUpdateAvailable()
	if available {
		t.Fatal("expected no update for dev builds")
	}
}

func TestFetchLatestReleaseNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	origURL := githubReleasesLatestURL
	githubReleasesLatestURL = srv.URL
	t.Cleanup(func() { githubReleasesLatestURL = origURL })

	tag, url, ok := fetchLatestRelease(nil)
	if ok || tag != "" || url != "" {
		t.Fatalf("expected failure on 404, got ok=%v tag=%q url=%q", ok, tag, url)
	}
}

func TestAdminUpdateData(t *testing.T) {
	app := &App{Version: "dev"}
	data := app.adminUpdateData()
	if data["UpdateAvailable"] != false {
		t.Fatalf("UpdateAvailable=%v", data["UpdateAvailable"])
	}
}
