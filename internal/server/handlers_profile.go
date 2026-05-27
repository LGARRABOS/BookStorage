package server

import (
	"bookstorage/internal/catalog"
	"bookstorage/internal/i18n"
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type profileUser struct {
	ID          int
	Username    string
	Password    sql.NullString
	GoogleSub   sql.NullString
	GoogleEmail sql.NullString
	DisplayName sql.NullString
	Email       sql.NullString
	Bio         sql.NullString
	AvatarPath  sql.NullString
	IsPublic    sql.NullInt64
}

// readingTimelineDay holds sparse daily aggregates for Chart.js (started / finished / last activity).
type readingTimelineDay struct {
	Day      string `json:"day"`
	Started  int    `json:"started"`
	Finished int    `json:"finished"`
	Reading  int    `json:"reading"`
}

// statusChartRow is one doughnut slice; Status is translated for the active locale.
type statusChartRow struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

func (a *App) readingTimelineForCharts(userID int) []readingTimelineDay {
	dayExpr := func(col string) string {
		if a.Settings.UsePostgres() {
			return `TO_CHAR(` + col + ` AT TIME ZONE 'UTC', 'YYYY-MM-DD')`
		}
		return `strftime('%Y-%m-%d', ` + col + `)`
	}
	type dayCounts struct {
		Started  int
		Finished int
		Reading  int
	}
	byDay := map[string]*dayCounts{}
	addQuery := func(col string, field func(*dayCounts, int)) {
		q := `SELECT ` + dayExpr(col) + ` AS d, COUNT(*) FROM works WHERE user_id = ? AND ` + col + ` IS NOT NULL GROUP BY d ORDER BY d`
		rows, qerr := a.DB.Query(q, userID)
		if qerr != nil {
			return
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var d string
			var n int
			if rows.Scan(&d, &n) != nil {
				continue
			}
			if byDay[d] == nil {
				byDay[d] = &dayCounts{}
			}
			field(byDay[d], n)
		}
	}
	addQuery("started_at", func(dc *dayCounts, n int) { dc.Started = n })
	addQuery("finished_at", func(dc *dayCounts, n int) { dc.Finished = n })
	radRows, rerr := a.DB.Query(`SELECT day, chapter_increments FROM reading_activity_daily WHERE user_id = ? ORDER BY day`, userID)
	if rerr == nil {
		defer func() { _ = radRows.Close() }()
		for radRows.Next() {
			var d string
			var n int
			if radRows.Scan(&d, &n) != nil {
				continue
			}
			if byDay[d] == nil {
				byDay[d] = &dayCounts{}
			}
			byDay[d].Reading = n
		}
	}

	sortedDays := make([]string, 0, len(byDay))
	for d := range byDay {
		sortedDays = append(sortedDays, d)
	}
	sort.Strings(sortedDays)
	out := make([]readingTimelineDay, 0, len(sortedDays))
	for _, d := range sortedDays {
		dc := byDay[d]
		out = append(out, readingTimelineDay{Day: d, Started: dc.Started, Finished: dc.Finished, Reading: dc.Reading})
	}
	return out
}

func utcCalendarDayFromNullFlexTime(n nullFlexTime) string {
	if !n.Valid || strings.TrimSpace(n.String) == "" {
		return ""
	}
	s := strings.TrimSpace(n.String)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format("2006-01-02")
	}
	// Match storage used for works.last_chapter_at (naive UTC in SQLite schema / timestamptz in Postgres).
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.UTC); err == nil {
		return t.UTC().Format("2006-01-02")
	}
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		if t, err := time.ParseInLocation("2006-01-02", s[:10], time.UTC); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

// adjustReadingActivityDaily adds delta (may be negative) to chapter_increments for one UTC calendar day, clamped at 0.
// Returns the number of rows SQLite/Postgres report as affected (used to detect “wrong day” for corrections).
func (a *App) adjustReadingActivityDaily(userID int, dayUTC string, delta int) int64 {
	if a.DB == nil || userID <= 0 || delta == 0 {
		return 0
	}
	dayUTC = strings.TrimSpace(dayUTC)
	if len(dayUTC) >= 10 {
		dayUTC = dayUTC[:10]
	}
	if dayUTC == "" {
		return 0
	}
	var qUpdate string
	if a.Settings != nil && a.Settings.UsePostgres() {
		qUpdate = `UPDATE reading_activity_daily SET chapter_increments = GREATEST(0, chapter_increments + ?) WHERE user_id = ? AND day = ?`
	} else {
		qUpdate = `UPDATE reading_activity_daily SET chapter_increments = MAX(0, chapter_increments + ?) WHERE user_id = ? AND day = ?`
	}
	res, err := a.DB.Exec(qUpdate, delta, userID, dayUTC)
	if err != nil {
		log.Printf("reading_activity_daily update: %v", err)
		return 0
	}
	nAff, _ := res.RowsAffected()
	if nAff > 0 {
		return nAff
	}
	if delta < 0 {
		return 0
	}
	if _, err := a.DB.Exec(`INSERT INTO reading_activity_daily (user_id, day, chapter_increments) VALUES (?, ?, ?)`, userID, dayUTC, delta); err != nil {
		log.Printf("reading_activity_daily insert: %v", err)
		return 0
	}
	return 1
}

// applyChapterDeltaToReadingStats syncs reading_activity_daily when the user edits chapter count: increases log on
// today's UTC bucket; decreases correct tallies even when last_chapter_at and the typo landed on different UTC days.
func (a *App) applyChapterDeltaToReadingStats(userID int, delta int, lastChapterAtBefore nullFlexTime) {
	if delta == 0 {
		return
	}
	if delta > 0 {
		a.adjustReadingActivityDaily(userID, time.Now().UTC().Format("2006-01-02"), delta)
		return
	}
	need := -delta // chapters to remove from the daily stats
	today := time.Now().UTC().Format("2006-01-02")
	primary := utcCalendarDayFromNullFlexTime(lastChapterAtBefore)
	if primary == "" {
		primary = today
	}

	// Typical fat-finger: one day holds the whole spike — prefer the newest day with enough count.
	var heavy string
	if err := a.DB.QueryRow(`
		SELECT day FROM reading_activity_daily
		WHERE user_id = ? AND chapter_increments >= ?
		ORDER BY day DESC LIMIT 1`, userID, need).Scan(&heavy); err == nil && strings.TrimSpace(heavy) != "" {
		if a.adjustReadingActivityDaily(userID, strings.TrimSpace(heavy), delta) > 0 {
			return
		}
	}
	if a.adjustReadingActivityDaily(userID, primary, delta) > 0 {
		return
	}
	if primary != today && a.adjustReadingActivityDaily(userID, today, delta) > 0 {
		return
	}
	var fallback string
	if err := a.DB.QueryRow(`
		SELECT day FROM reading_activity_daily
		WHERE user_id = ? AND chapter_increments > 0
		ORDER BY chapter_increments DESC, day DESC
		LIMIT 1`, userID).Scan(&fallback); err == nil && strings.TrimSpace(fallback) != "" {
		if a.adjustReadingActivityDaily(userID, strings.TrimSpace(fallback), delta) > 0 {
			return
		}
	}
	log.Printf("reading_activity_daily: chapter correction delta %d not applied for user %d (no matching day row)", delta, userID)
}

// recordReadingChapterIncrements adds a positive delta to today's UTC rollup (+ button, etc.).
func (a *App) recordReadingChapterIncrements(userID int, delta int) {
	if delta <= 0 {
		return
	}
	a.adjustReadingActivityDaily(userID, time.Now().UTC().Format("2006-01-02"), delta)
}

func (a *App) statusDistribForCharts(userID int, tr i18n.Translations) []statusChartRow {
	var out []statusChartRow
	sRows, err := a.DB.Query(`SELECT COALESCE(status, ''), COUNT(*) FROM works WHERE user_id = ? GROUP BY status ORDER BY COUNT(*) DESC`, userID)
	if err != nil {
		return out
	}
	defer func() { _ = sRows.Close() }()
	for sRows.Next() {
		var raw string
		var c int
		if sRows.Scan(&raw, &c) != nil {
			continue
		}
		out = append(out, statusChartRow{Status: i18n.TranslateStatus(raw, tr), Count: c})
	}
	return out
}

func deleteMediaFile(folder, storedPath string) {
	if strings.TrimSpace(storedPath) == "" {
		return
	}
	filename := filepath.Base(storedPath)
	if filename == "" {
		return
	}
	target := filepath.Join(folder, filename)
	_ = os.Remove(target)
}

func (a *App) renderProfilePage(w http.ResponseWriter, r *http.Request, userID int, extra map[string]any) {
	var u profileUser
	if v, ok := extra["User"].(profileUser); ok {
		u = v
	} else {
		err := a.DB.QueryRow(
			`SELECT id, username, password, google_sub, google_email, display_name, email, bio, avatar_path, is_public
			 FROM users WHERE id = ?`,
			userID,
		).Scan(
			&u.ID, &u.Username, &u.Password, &u.GoogleSub, &u.GoogleEmail,
			&u.DisplayName, &u.Email, &u.Bio, &u.AvatarPath, &u.IsPublic,
		)
		if err != nil {
			http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
			return
		}
	}

	var totalWorks int
	_ = a.DB.QueryRow(`SELECT COUNT(*) FROM works WHERE user_id = ?`, userID).Scan(&totalWorks)
	var totalChapters int
	_ = a.DB.QueryRow(`SELECT COALESCE(SUM(chapter), 0) FROM works WHERE user_id = ?`, userID).Scan(&totalChapters)
	var completedCount int
	_ = a.DB.QueryRow(`SELECT COUNT(*) FROM works WHERE user_id = ? AND (status = 'Terminé' OR status = 'Completed')`, userID).Scan(&completedCount)
	var readingCount int
	_ = a.DB.QueryRow(`SELECT COUNT(*) FROM works WHERE user_id = ? AND (status = 'En cours' OR status = 'Reading')`, userID).Scan(&readingCount)

	sessions, _ := a.listActiveSessions(userID)
	apiTokens, _ := a.listAPITokens(userID)
	webhooks, _ := a.listWebhookEndpoints(userID)
	passkeys, _ := a.listWebAuthnCredentials(userID)
	_, tok, _ := a.currentSession(r)
	currentSessionHash := ""
	if tok != "" {
		currentSessionHash = hashSessionToken(tok)
	}
	blocklist, _ := catalog.LoadUserBlocklist(a.DB, int64(userID))
	q := r.URL.Query()
	data := map[string]any{
		"User":               u,
		"TotalWorks":         totalWorks,
		"TotalChapters":      totalChapters,
		"CompletedCount":     completedCount,
		"ReadingCount":       readingCount,
		"Sessions":           sessions,
		"CurrentSession":     currentSessionHash,
		"APITokens":          apiTokens,
		"Webhooks":           webhooks,
		"WebAuthnPasskeys":   passkeys,
		"BlocklistGenres":    blocklist.Genres,
		"BlocklistTags":      blocklist.Tags,
		"BlocklistAdded":     q.Get("blocklist_added") == "1",
		"BlocklistRemoved":   q.Get("blocklist_removed") == "1",
		"BlocklistError":     q.Get("blocklist_error") == "1",
		"LogoutAllDone":      q.Get("logout_all") == "1",
		"GoogleLinked":       q.Get("google_linked") == "1",
		"GoogleUnlinked":     q.Get("google_unlinked") == "1",
		"GoogleOAuthError":   strings.TrimSpace(q.Get("google_error")),
		"ReadingStatsReset":  strings.TrimSpace(q.Get("reading_stats_reset")),
		"APITokenRevoked":    q.Get("api_token_revoked") == "1",
		"WebhookUpdated":     q.Get("webhook_updated") == "1",
		"WebhookDeleted":     q.Get("webhook_deleted") == "1",
		"WebhookTestSent":    q.Get("webhook_test") == "1",
		"WebhookError":       q.Get("webhook_error") == "1",
		"WebAuthnDeleted":    q.Get("webauthn_deleted") == "1",
		"WebAuthnRegistered": q.Get("webauthn_registered") == "1",
		"WebAuthnError":      strings.TrimSpace(q.Get("webauthn_error")),
	}
	for k, v := range extra {
		data[k] = v
	}
	a.renderTemplate(w, r, "profile", a.mergeData(r, data))
}

func (a *App) HandleProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}

	var u profileUser
	err := a.DB.QueryRow(
		`SELECT id, username, password, google_sub, google_email, display_name, email, bio, avatar_path, is_public
         FROM users WHERE id = ?`,
		userID,
	).Scan(
		&u.ID,
		&u.Username,
		&u.Password,
		&u.GoogleSub,
		&u.GoogleEmail,
		&u.DisplayName,
		&u.Email,
		&u.Bio,
		&u.AvatarPath,
		&u.IsPublic,
	)
	if err != nil {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.renderProfilePage(w, r, userID, map[string]any{"User": u})
	case http.MethodPost:
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		newUsername := strings.TrimSpace(r.FormValue("username"))
		displayName := strings.TrimSpace(r.FormValue("display_name"))
		email := strings.TrimSpace(r.FormValue("email"))
		bio := strings.TrimSpace(r.FormValue("bio"))
		currentPassword := r.FormValue("current_password")
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_password")
		visibility := r.FormValue("is_public")

		if newUsername == "" {
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}

		updates := map[string]any{}
		requirePasswordCheck := false

		if newUsername != u.Username {
			requirePasswordCheck = true
			updates["username"] = newUsername
		}

		hasLocalPassword := u.Password.Valid && strings.TrimSpace(u.Password.String) != ""

		if newPassword != "" || confirmPassword != "" {
			if hasLocalPassword {
				requirePasswordCheck = true
			}
			if newPassword != confirmPassword {
				http.Redirect(w, r, "/profile", http.StatusFound)
				return
			}
			if len(newPassword) < 8 {
				http.Redirect(w, r, "/profile", http.StatusFound)
				return
			}
			hashedPassword, err := hashPassword(newPassword)
			if err != nil {
				http.Redirect(w, r, "/profile", http.StatusFound)
				return
			}
			updates["password"] = hashedPassword
		}

		if requirePasswordCheck {
			if hasLocalPassword {
				if currentPassword == "" || !verifyPassword(u.Password.String, currentPassword) {
					http.Redirect(w, r, "/profile", http.StatusFound)
					return
				}
			}
			// Compte sans mot de passe local : changement de pseudo ou premier mot de passe sans "mot de passe actuel".
		}

		if displayName != "" {
			updates["display_name"] = displayName
		} else {
			updates["display_name"] = nil
		}

		if email != "" {
			updates["email"] = email
		} else {
			updates["email"] = nil
		}

		if bio != "" {
			updates["bio"] = bio
		} else {
			updates["bio"] = nil
		}

		if visibility != "" {
			if visibility == "1" {
				updates["is_public"] = 1
			} else {
				updates["is_public"] = 0
			}
		}

		previousAvatar := u.AvatarPath.String
		newAvatarPath := previousAvatar

		if rel, err := saveImageFromForm(r, "avatar", a.Settings.ProfileUploadFolder, a.Settings.ProfileUploadURLPath, userID); err == nil {
			newAvatarPath = rel
			updates["avatar_path"] = newAvatarPath
		}

		if len(updates) == 0 {
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}

		setParts := make([]string, 0, len(updates))
		args := make([]any, 0, len(updates)+1)
		for k, v := range updates {
			setParts = append(setParts, k+" = ?")
			args = append(args, v)
		}
		args = append(args, userID)

		stmt := "UPDATE users SET " + strings.Join(setParts, ", ") + " WHERE id = ?"
		if _, err := a.DB.Exec(stmt, args...); err != nil {
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}

		if newAvatarPath != "" && previousAvatar != "" && previousAvatar != newAvatarPath {
			var remaining int
			if err := a.DB.QueryRow(
				`SELECT COUNT(*) FROM users WHERE avatar_path = ? AND id != ?`,
				previousAvatar, userID,
			).Scan(&remaining); err == nil && remaining == 0 {
				deleteMediaFile(a.Settings.ProfileUploadFolder, previousAvatar)
			}
		}

		http.Redirect(w, r, "/profile", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) HandleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}
	currentPassword := r.FormValue("current_password")
	confirmText := strings.TrimSpace(r.FormValue("confirm_delete"))
	if strings.ToUpper(confirmText) != "SUPPRIMER" {
		http.Redirect(w, r, "/profile?delete_error=1", http.StatusFound)
		return
	}

	var storedPassword sql.NullString
	var avatarPath sql.NullString
	var googleSub sql.NullString
	err := a.DB.QueryRow(
		`SELECT password, avatar_path, google_sub FROM users WHERE id = ?`,
		userID,
	).Scan(&storedPassword, &avatarPath, &googleSub)
	if err != nil {
		http.Redirect(w, r, "/profile?delete_error=1", http.StatusFound)
		return
	}
	hasLocalPassword := storedPassword.Valid && strings.TrimSpace(storedPassword.String) != ""
	googleLinked := googleSub.Valid && strings.TrimSpace(googleSub.String) != ""
	isGoogleOnly := !hasLocalPassword && googleLinked
	if isGoogleOnly {
		if strings.TrimSpace(currentPassword) != "" {
			http.Redirect(w, r, "/profile?delete_error=1", http.StatusFound)
			return
		}
	} else {
		if currentPassword == "" || !verifyPassword(storedPassword.String, currentPassword) {
			http.Redirect(w, r, "/profile?delete_error=1", http.StatusFound)
			return
		}
	}

	type imgPathRow struct {
		imagePath sql.NullString
	}
	var workImagePaths []string
	rows, err := a.DB.Query(`SELECT image_path FROM works WHERE user_id = ?`, userID)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var p imgPathRow
			if rows.Scan(&p.imagePath) == nil && p.imagePath.Valid {
				workImagePaths = append(workImagePaths, p.imagePath.String)
			}
		}
	}

	tx, err := a.DB.Begin()
	if err != nil {
		http.Redirect(w, r, "/profile?delete_error=1", http.StatusFound)
		return
	}
	if _, err := tx.Exec(`DELETE FROM works WHERE user_id = ?`, userID); err != nil {
		_ = tx.Rollback()
		http.Redirect(w, r, "/profile?delete_error=1", http.StatusFound)
		return
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE id = ?`, userID); err != nil {
		_ = tx.Rollback()
		http.Redirect(w, r, "/profile?delete_error=1", http.StatusFound)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Redirect(w, r, "/profile?delete_error=1", http.StatusFound)
		return
	}

	if avatarPath.Valid {
		deleteMediaFile(a.Settings.ProfileUploadFolder, avatarPath.String)
	}
	for _, p := range workImagePaths {
		deleteMediaFile(a.Settings.UploadFolder, p)
	}

	a.clearSession(w)
	http.Redirect(w, r, "/?account_deleted=1", http.StatusFound)
}
