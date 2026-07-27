package server

import (
	"bytes"
	"encoding/csv"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseBdgestCSVRecords(t *testing.T) {
	csvData := "" +
		"IdAlbum;ISBN;Serie;Num;NumA;Titre;Editeur;Collection;EO;DL;AI;Cote;Etat;DateAchat;PrixAchat;Note;Scenariste;Dessinateur;Wishlist;AVendre;Perso1;Perso2;Perso3;Perso4;Format;Suivi;Commentaire;Table\n" +
		"12345;9782205070000;Astérix;7;;Le combat des chefs;Dargaud;;;;;;;2020-01-01;12.50;8;Goscinny;Uderzo;0;0;;;;;Album;1;Super album;\n" +
		"999;9782205070001;Blake et Mortimer;;HS;La marque jaune;Blake et Mortimer;;;;;;;2021-02-02;15;10;Jacobs;Jacobs;1;0;;;;;Album;0;Wishlist item;\n"

	cr := csv.NewReader(strings.NewReader(csvData))
	cr.Comma = ';'
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	records, err := cr.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := parseBdCSVRecords(records)
	if !ok || len(rows) != 2 {
		t.Fatalf("ok=%v len=%d", ok, len(rows))
	}

	got := rows[0]
	if got.Title != "Astérix — Le combat des chefs" {
		t.Fatalf("title=%q", got.Title)
	}
	if got.ISBN != "9782205070000" {
		t.Fatalf("isbn=%q", got.ISBN)
	}
	if got.Tome != 7 || got.Status != "Terminé" || got.Source != "bdgest" || got.ExternalID != "12345" {
		t.Fatalf("owned row: %+v", got)
	}
	if got.Rating != 4 { // MAL-style 8/10 → 4★
		t.Fatalf("rating=%d want 4", got.Rating)
	}
	if got.Notes != "Super album" {
		t.Fatalf("notes=%q", got.Notes)
	}

	wish := rows[1]
	if wish.Status != "À lire" || wish.ExternalID != "999" {
		t.Fatalf("wishlist row: %+v", wish)
	}
	if wish.Rating != 5 { // 10/10 → 5★
		t.Fatalf("wishlist rating=%d want 5", wish.Rating)
	}
}

func TestParseBdgestCSV_BOMAndAAcheter(t *testing.T) {
	csvData := "\ufeffIdAlbum;Serie;Num;Titre;Note;AAcheter;Commentaire\n" +
		"42;Spirou;1;La ville immergée;6;1;À acheter\n"
	cr := csv.NewReader(strings.NewReader(csvData))
	cr.Comma = ';'
	cr.FieldsPerRecord = -1
	records, err := cr.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := parseBdCSVRecords(records)
	if !ok || len(rows) != 1 {
		t.Fatalf("ok=%v rows=%v", ok, rows)
	}
	if rows[0].Status != "À lire" || rows[0].Title != "Spirou — La ville immergée" {
		t.Fatalf("got %+v", rows[0])
	}
}

func TestParseBdgestPositionalCSV(t *testing.T) {
	// IdAlbum;ISBN;Serie;Num;NumA;Titre;… Note@15 … Wishlist@18 … Format@24 … Commentaire@26
	row := make([]string, 27)
	row[0] = "777"
	row[1] = "9782205070999"
	row[2] = "Thorgal"
	row[3] = "1"
	row[5] = "La magicienne trahie"
	row[15] = "9"
	row[18] = "0"
	row[24] = "Album"
	row[26] = "Classique"
	records := [][]string{row}
	rows, ok := parseBdCSVRecords(records)
	if !ok || len(rows) != 1 {
		t.Fatalf("ok=%v len=%d", ok, len(rows))
	}
	if rows[0].Title != "Thorgal — La magicienne trahie" || rows[0].Tome != 1 || rows[0].ExternalID != "777" {
		t.Fatalf("got %+v", rows[0])
	}
	if rows[0].ISBN != "9782205070999" {
		t.Fatalf("isbn=%q", rows[0].ISBN)
	}
	if rows[0].Rating != 5 { // 9/10 → (9+1)/2 = 5
		t.Fatalf("rating=%d", rows[0].Rating)
	}
}

func TestParseBdCSV_BookStorageStillWorks(t *testing.T) {
	csvData := "Title;Tome;Status;Type;Rating;Source\nTintin;3;En cours;Série;4;manual\n"
	cr := csv.NewReader(strings.NewReader(csvData))
	cr.Comma = ';'
	cr.FieldsPerRecord = -1
	records, err := cr.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := parseBdCSVRecords(records)
	if !ok || len(rows) != 1 || rows[0].Title != "Tintin" || rows[0].Source != "manual" {
		t.Fatalf("ok=%v rows=%+v", ok, rows)
	}
}

func TestImportFromCSV_Bdgest(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.WriteField("duplicate_mode", "skip")
	part, err := w.CreateFormFile("import_file", "bdgest.csv")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, "IdAlbum;ISBN;Serie;Num;NumA;Titre;Note;Wishlist;Commentaire\n"+
		"55;9782;Lucky Luke;12;;Les collines noires;7;0;Western\n")
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/bd/import", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleBdImport(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}

	var title, status, source, extID, notes, isbn string
	var tome, rating int
	err = db.QueryRow(
		`SELECT title, tome, status, rating, notes, source, COALESCE(external_id, ''), COALESCE(isbn, '') FROM bd_works WHERE user_id = 1 LIMIT 1`,
	).Scan(&title, &tome, &status, &rating, &notes, &source, &extID, &isbn)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Lucky Luke — Les collines noires" || tome != 12 || status != "Terminé" || source != "bdgest" || extID != "55" {
		t.Fatalf("got title=%q tome=%d status=%q source=%q ext=%q", title, tome, status, source, extID)
	}
	if isbn != "9782" {
		t.Fatalf("isbn=%q", isbn)
	}
	if rating != 4 || notes != "Western" { // 7/10 → 4★
		t.Fatalf("rating=%d notes=%q", rating, notes)
	}
}
