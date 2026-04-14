package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"bookstorage/internal/config"
)

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
