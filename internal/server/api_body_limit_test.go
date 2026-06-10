package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeAPIJSONBodyRejectsOversizedPayload(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	largeTitle := strings.Repeat("a", maxAPIJSONBodyBytes)
	body, err := json.Marshal(map[string]any{"title": largeTitle})
	if err != nil {
		t.Fatal(err)
	}
	if len(body) <= maxAPIJSONBodyBytes {
		t.Fatalf("payload len %d, want > %d", len(body), maxAPIJSONBodyBytes)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/works", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.HandleAPIWorksCreate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	var payload map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["error"] != "invalid_json" {
		t.Fatalf("error %q, want invalid_json", payload["error"])
	}
}

func TestDecodeAPIJSONBodyAcceptsNormalPayload(t *testing.T) {
	w := httptest.NewRecorder()
	body := strings.NewReader(`{"title":"ok"}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	var got struct {
		Title string `json:"title"`
	}
	if err := decodeAPIJSONBody(w, req, &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != "ok" {
		t.Fatalf("title %q", got.Title)
	}
	_, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
}
