package server

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCsvSafeCell(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"normal", "normal"},
		{"=1+1", "'=1+1"},
		{"+cmd", "'+cmd"},
		{"-2+3", "'-2+3"},
		{"@SUM(A1)", "'@SUM(A1)"},
		{"\ttab", "'\ttab"},
		{"\rCR", "'\rCR"},
	}
	for _, tc := range cases {
		if got := csvSafeCell(tc.in); got != tc.want {
			t.Fatalf("csvSafeCell(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHandleExportCSVFormulaInjection(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	_, err := db.Exec(
		`INSERT INTO works (title, chapter, link, notes, user_id, status, reading_type)
		 VALUES ('=HYPERLINK("http://evil")', 1, '+evil', '+note', 1, 'En cours', 'Manga')`,
	)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/export", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleExport(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.Bytes()
	if len(body) < 3 {
		t.Fatal("expected UTF-8 BOM")
	}
	r := csv.NewReader(bytes.NewReader(body[3:]))
	r.Comma = ';'
	records, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) < 2 {
		t.Fatalf("records: %v", records)
	}
	row := records[1]
	if !strings.HasPrefix(row[0], "'=") {
		t.Fatalf("title not escaped: %q", row[0])
	}
	if !strings.HasPrefix(row[2], "'+") {
		t.Fatalf("link not escaped: %q", row[2])
	}
	if !strings.HasPrefix(row[6], "'+") {
		t.Fatalf("notes not escaped: %q", row[6])
	}
}
