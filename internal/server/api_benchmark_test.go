package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"bookstorage/internal/config"
	"bookstorage/internal/database"
)

func BenchmarkHandleAPIWorksList_Search(b *testing.B) {
	dir := b.TempDir()
	s := &config.Settings{
		Database:             filepath.Join(dir, "db.sqlite"),
		SecretKey:            "0123456789abcdef0123456789abcdef",
		Environment:          "development",
		SuperadminUsername:   "admin",
		SuperadminPassword:   "TestAdmin!99",
		DataDirectory:        dir,
		UploadFolder:         filepath.Join(dir, "img"),
		ProfileUploadFolder:  filepath.Join(dir, "av"),
		UploadURLPath:        "images",
		ProfileUploadURLPath: "avatars",
	}
	db, err := database.Open(s)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	if err := database.EnsureSchema(db, s); err != nil {
		b.Fatal(err)
	}

	app := &App{Settings: s, DB: db}

	tx, err := db.Begin()
	if err != nil {
		b.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO works (title, chapter, status, reading_type, rating, notes, user_id, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 1500; i++ {
		title := "Title " + strconv.Itoa(i)
		notes := "notes " + strconv.Itoa(i)
		if i%10 == 0 {
			title = "Alpha " + strconv.Itoa(i)
			notes = "alpha keyword " + strconv.Itoa(i)
		}
		if _, err := stmt.Exec(title, i%200, "En cours", "Manga", i%5, notes, 1); err != nil {
			b.Fatal(err)
		}
	}
	if err := stmt.Close(); err != nil {
		b.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}

	session := app.signSession(1, time.Now().Add(time.Hour).Unix())
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/works?search=alpha&limit=20&page=1&sort=recent", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: session})
		rec := httptest.NewRecorder()
		app.HandleAPIWorksList(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"data"`) {
			b.Fatalf("unexpected payload: %s", rec.Body.String())
		}
	}
}
