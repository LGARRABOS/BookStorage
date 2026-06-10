package server

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestAdminAccountActions_requirePOST(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	adminToken := mustCreateSession(t, app, 1)

	_, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_admin) VALUES ('pendinguser', 'x', 0, 0)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	var pendingID int
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'pendinguser'`).Scan(&pendingID); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"approve GET", http.MethodGet, "/admin/approve/" + strconv.Itoa(pendingID)},
		{"promote GET", http.MethodGet, "/admin/promote/" + strconv.Itoa(pendingID)},
		{"delete GET", http.MethodGet, "/admin/delete_account/" + strconv.Itoa(pendingID)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.SetPathValue("id", strconv.Itoa(pendingID))
			req.AddCookie(&http.Cookie{Name: "session", Value: adminToken})
			rec := httptest.NewRecorder()
			switch {
			case strings.Contains(tc.path, "approve"):
				app.HandleApproveAccount(rec, req)
			case strings.Contains(tc.path, "promote"):
				app.HandlePromoteAccount(rec, req)
			default:
				app.HandleDeleteAccount(rec, req)
			}
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status %d, want 405", rec.Code)
			}
		})
	}
}

func TestHandleApproveAccount_POSTWithCSRF(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	adminToken := mustCreateSession(t, app, 1)

	_, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_admin) VALUES ('approveme', 'x', 0, 0)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	var uid int
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'approveme'`).Scan(&uid); err != nil {
		t.Fatal(err)
	}

	handler := app.WithRequestPolicies(http.HandlerFunc(app.RequireAdmin(app.HandleApproveAccount)))

	req := httptest.NewRequest(http.MethodPost, "/admin/approve/"+strconv.Itoa(uid), nil)
	req.SetPathValue("id", strconv.Itoa(uid))
	req.Host = "books.test"
	req.Header.Set("Origin", "http://books.test")
	req.AddCookie(&http.Cookie{Name: "session", Value: adminToken})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d, want 302", rec.Code)
	}

	var validated int
	if err := db.QueryRow(`SELECT validated FROM users WHERE id = ?`, uid).Scan(&validated); err != nil {
		t.Fatal(err)
	}
	if validated != 1 {
		t.Fatalf("validated=%d, want 1", validated)
	}
}

func TestHandleDeleteAccount_adminCannotDeleteOtherAdmin(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	_, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_admin, is_superadmin)
		 VALUES ('plainadmin', 'x', 1, 1, 0)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	var plainAdminID int
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'plainadmin'`).Scan(&plainAdminID); err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(
		`INSERT INTO users (username, password, validated, is_admin, is_superadmin)
		 VALUES ('otheradmin', 'x', 1, 1, 0)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	var targetAdminID int
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'otheradmin'`).Scan(&targetAdminID); err != nil {
		t.Fatal(err)
	}

	plainToken := mustCreateSession(t, app, plainAdminID)
	req := httptest.NewRequest(http.MethodPost, "/admin/delete_account/"+strconv.Itoa(targetAdminID), nil)
	req.SetPathValue("id", strconv.Itoa(targetAdminID))
	req.AddCookie(&http.Cookie{Name: "session", Value: plainToken})
	rec := httptest.NewRecorder()
	app.HandleDeleteAccount(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d, want 302", rec.Code)
	}

	var stillThere int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, targetAdminID).Scan(&stillThere); err != nil {
		t.Fatal(err)
	}
	if stillThere != 1 {
		t.Fatalf("other admin should not be deleted by plain admin")
	}
}

func TestHandleDeleteAccount_superadminCanDeleteAdmin(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	_, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_admin, is_superadmin)
		 VALUES ('deleteme', 'x', 1, 1, 0)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	var targetAdminID int
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'deleteme'`).Scan(&targetAdminID); err != nil {
		t.Fatal(err)
	}

	var superID int
	if err := db.QueryRow(`SELECT id FROM users WHERE is_superadmin = 1 LIMIT 1`).Scan(&superID); err != nil {
		t.Fatal(err)
	}
	superToken := mustCreateSession(t, app, superID)

	req := httptest.NewRequest(http.MethodPost, "/admin/delete_account/"+strconv.Itoa(targetAdminID), nil)
	req.SetPathValue("id", strconv.Itoa(targetAdminID))
	req.AddCookie(&http.Cookie{Name: "session", Value: superToken})
	rec := httptest.NewRecorder()
	app.HandleDeleteAccount(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d, want 302", rec.Code)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, targetAdminID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("superadmin should delete admin account")
	}
}

func TestHandleApproveAccount_POSTBlocksForeignOrigin(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	adminToken := mustCreateSession(t, app, 1)

	handler := app.WithRequestPolicies(http.HandlerFunc(app.RequireAdmin(app.HandleApproveAccount)))

	req := httptest.NewRequest(http.MethodPost, "/admin/approve/2", nil)
	req.SetPathValue("id", "2")
	req.Host = "books.test"
	req.Header.Set("Origin", "https://evil.example")
	req.AddCookie(&http.Cookie{Name: "session", Value: adminToken})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", rec.Code)
	}
}
