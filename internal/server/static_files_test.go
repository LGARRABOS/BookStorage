package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bookstorage/internal/config"
)

func testRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Join(dir, "..")
	}
	t.Fatal("go.mod not found from test working directory")
	return ""
}

func TestBundledPWAManifestIconPathsExist(t *testing.T) {
	root := testRepoRoot(t)
	manifestPath := filepath.Join(root, "static", "pwa", "manifest.json")
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Icons []struct {
			Src string `json:"src"`
		} `json:"icons"`
		Shortcuts []struct {
			Icons []struct {
				Src string `json:"src"`
			} `json:"icons"`
		} `json:"shortcuts"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, ic := range m.Icons {
		paths = append(paths, ic.Src)
	}
	for _, sc := range m.Shortcuts {
		for _, ic := range sc.Icons {
			paths = append(paths, ic.Src)
		}
	}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if !strings.HasPrefix(p, "/static/") {
			t.Fatalf("unexpected icon path %q", p)
		}
		rel := strings.TrimPrefix(p, "/static/")
		full := filepath.Join(root, "static", filepath.FromSlash(rel))
		if _, err := os.Stat(full); err != nil {
			t.Fatalf("missing bundled file for manifest icon %q: %v", p, err)
		}
	}
}

func TestStaticFilesHandler_ServesAvatarFromProfileFolder(t *testing.T) {
	dir := t.TempDir()
	avDir := filepath.Join(dir, "custom_avatars")
	if err := os.MkdirAll(avDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(dir, "static")
	if err := os.MkdirAll(filepath.Join(bundle, "css"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "css", "x.css"), []byte("body{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(avDir, "7_pic.png"), []byte("fakepng"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &config.Settings{
		DataDirectory:        dir,
		WebStaticRoot:        bundle,
		ProfileUploadFolder:  avDir,
		ProfileUploadURLPath: "avatars",
		UploadFolder:         filepath.Join(dir, "img"),
		UploadURLPath:        "images",
	}
	h := StaticFilesHandler(s)

	t.Run("avatar", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/static/avatars/7_pic.png", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		if got := rec.Body.String(); got != "fakepng" {
			t.Fatalf("body %q", got)
		}
	})

	t.Run("bundle", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/static/css/x.css", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
	})
}
