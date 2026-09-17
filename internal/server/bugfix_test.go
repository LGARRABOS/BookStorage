package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func TestImportBdDuplicate_sameTitleDifferentTome(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	report := ImportReport{}
	app.importOneBdWork(1, 1, exportBdWork{Title: "Spirou", Tome: 1, Status: "Terminé", BdType: "Série"}, DuplicateUpdate, &report)
	app.importOneBdWork(1, 2, exportBdWork{Title: "Spirou", Tome: 2, Status: "En cours", BdType: "Série"}, DuplicateUpdate, &report)
	if report.Imported != 2 {
		t.Fatalf("expected 2 imported volumes, report=%+v", report)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bd_works WHERE user_id = 1 AND title = 'Spirou'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 rows, got %d", n)
	}
}

func TestImportWorkDuplicate_sameTitleDifferentLink(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	report := ImportReport{}
	app.importOneWork(1, 1, exportWork{Title: "Naruto", Status: "En cours", ReadingType: "Manga", Link: "https://a.example/n"}, DuplicateUpdate, &report)
	app.importOneWork(1, 2, exportWork{Title: "Naruto", Status: "En cours", ReadingType: "Manga", Link: "https://b.example/n"}, DuplicateUpdate, &report)
	if report.Imported != 2 {
		t.Fatalf("expected 2 imported works, report=%+v", report)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM works WHERE user_id = 1 AND title = 'Naruto'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 rows, got %d", n)
	}
}

func TestHandleDecrement_adjustsReadingActivity(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	res, err := db.Exec(`INSERT INTO works (title, chapter, user_id, status) VALUES ('W', 0, 1, 'En cours')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	idPath := strconv.FormatInt(id, 10)

	post := func(h http.HandlerFunc, path string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.SetPathValue("id", idPath)
		req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status %d", path, rec.Code)
		}
	}
	post(app.HandleIncrement, "/api/increment/"+idPath)
	post(app.HandleDecrement, "/api/decrement/"+idPath)

	var n int
	if err := db.QueryRow(`SELECT COALESCE(SUM(chapter_increments), 0) FROM reading_activity_daily WHERE user_id = 1`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected daily increments corrected to 0, got %d", n)
	}
}

func TestHandleDeleteProfile_withRelatedModuleRows(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	hashed, err := hashPassword("OldPass!99")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO users (id, username, password, validated) VALUES (20, 'deleteme', ?, 1)`,
		hashed,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO works (title, chapter, user_id, status) VALUES ('W', 1, 20, 'En cours')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO anime_works (title, episode, status, anime_type, user_id) VALUES ('A', 1, 'En cours', 'TV', 20)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO bd_works (title, tome, status, bd_type, user_id) VALUES ('B', 1, 'À lire', 'Série', 20)`); err != nil {
		t.Fatal(err)
	}

	form := url.Values{
		"current_password": {"OldPass!99"},
		"confirm_delete":   {"SUPPRIMER"},
	}
	req := httptest.NewRequest(http.MethodPost, "/profile/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 20)})
	rec := httptest.NewRecorder()
	app.HandleDeleteProfile(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}

	var users, anime, bd int
	_ = db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = 20`).Scan(&users)
	_ = db.QueryRow(`SELECT COUNT(*) FROM anime_works WHERE user_id = 20`).Scan(&anime)
	_ = db.QueryRow(`SELECT COUNT(*) FROM bd_works WHERE user_id = 20`).Scan(&bd)
	if users != 0 || anime != 0 || bd != 0 {
		t.Fatalf("expected cascade delete, users=%d anime=%d bd=%d", users, anime, bd)
	}
}

func TestHandleMergeDuplicate_reassignsChildren(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	resFrom, err := db.Exec(`INSERT INTO works (title, chapter, user_id, status, reading_type) VALUES ('Series', 1, 1, 'En cours', 'Manga')`)
	if err != nil {
		t.Fatal(err)
	}
	fromID, _ := resFrom.LastInsertId()
	resInto, err := db.Exec(`INSERT INTO works (title, chapter, user_id, status, reading_type) VALUES ('Series', 3, 1, 'En cours', 'Manga')`)
	if err != nil {
		t.Fatal(err)
	}
	intoID, _ := resInto.LastInsertId()
	resChild, err := db.Exec(
		`INSERT INTO works (title, chapter, user_id, status, reading_type, parent_work_id) VALUES ('Series child', 0, 1, 'En cours', 'Manga', ?)`,
		fromID,
	)
	if err != nil {
		t.Fatal(err)
	}
	childID, _ := resChild.LastInsertId()

	form := url.Values{
		"from_id": {strconv.FormatInt(fromID, 10)},
		"into_id": {strconv.FormatInt(intoID, 10)},
	}
	req := httptest.NewRequest(http.MethodPost, pathToolsMangaDup, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleMergeDuplicate(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}

	var leftover int
	_ = db.QueryRow(`SELECT COUNT(*) FROM works WHERE id = ?`, fromID).Scan(&leftover)
	if leftover != 0 {
		t.Fatalf("source work still present")
	}
	var parent int
	if err := db.QueryRow(`SELECT COALESCE(parent_work_id, 0) FROM works WHERE id = ?`, childID).Scan(&parent); err != nil {
		t.Fatal(err)
	}
	if parent != int(intoID) {
		t.Fatalf("child parent_work_id=%d want %d", parent, intoID)
	}
}

func TestTakeWebAuthnChallenge_oneShot(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	key, err := app.putWebAuthnChallenge(&webauthn.SessionData{Challenge: "abc"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, uid, ok := app.takeWebAuthnChallenge(key)
	if !ok || uid != 1 {
		t.Fatalf("first take ok=%v uid=%d", ok, uid)
	}
	_, _, ok = app.takeWebAuthnChallenge(key)
	if ok {
		t.Fatal("challenge should not be reusable")
	}
}

func TestListActiveSessions_respectsAbsoluteTTL(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	token := mustCreateSession(t, app, 1)
	created := time.Now().UTC().Add(-sessionAbsoluteTTL - time.Hour)
	expires := time.Now().UTC().Add(time.Hour)
	if _, err := db.Exec(
		`UPDATE sessions SET created_at = ?, expires_at = ? WHERE token_hash = ?`,
		created, expires, hashSessionToken(token),
	); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	if _, _, ok := app.currentSession(req); ok {
		t.Fatal("auth should reject absolute-expired session")
	}
	list, err := app.listActiveSessions(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("listActiveSessions should hide absolute-expired session, got %d", len(list))
	}
}
