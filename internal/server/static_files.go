package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"bookstorage/internal/config"
)

// StaticFilesHandler serves GET/HEAD /static/*.
// User avatars and work images are read from ProfileUploadFolder / UploadFolder (even when absolute paths),
// so URLs stay under /static/<urlPath>/…; other paths fall back to the bundled WebStaticRoot (CSS, JS, icons).
func StaticFilesHandler(s *config.Settings) http.Handler {
	bundleRoot := strings.TrimSpace(s.WebStaticRoot)
	if bundleRoot == "" {
		bundleRoot = filepath.Join(s.DataDirectory, "static")
	}
	bundle := http.FileServer(http.Dir(bundleRoot))
	avPrefix := strings.Trim(strings.ReplaceAll(s.ProfileUploadURLPath, "\\", "/"), "/")
	imgPrefix := strings.Trim(strings.ReplaceAll(s.UploadURLPath, "\\", "/"), "/")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		rel := strings.TrimPrefix(r.URL.Path, "/static/")
		rel = strings.Trim(rel, "/")
		rel = strings.ReplaceAll(rel, "\\", "/")
		if rel == "" || strings.Contains(rel, "..") {
			http.NotFound(w, r)
			return
		}

		tryUpload := func(dir, prefix string) bool {
			if prefix == "" || dir == "" {
				return false
			}
			p := prefix + "/"
			if !strings.HasPrefix(rel, p) {
				return false
			}
			name := strings.TrimPrefix(rel, p)
			if name == "" || strings.Contains(name, "/") {
				return false
			}
			base := filepath.Base(name)
			if base == "." || base == ".." {
				return false
			}
			full := filepath.Join(dir, base)
			fi, err := os.Stat(full)
			if err != nil || fi.IsDir() {
				return false
			}
			http.ServeFile(w, r, full)
			return true
		}

		if tryUpload(s.ProfileUploadFolder, avPrefix) {
			return
		}
		if tryUpload(s.UploadFolder, imgPrefix) {
			return
		}

		http.StripPrefix("/static/", bundle).ServeHTTP(w, r)
	})
}
