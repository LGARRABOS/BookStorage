package server

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bookstorage/internal/catalog"
)

const csvImportMaxBytes = 512 * 1024
const csvImportMaxRows = 2000

func randomHexID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// HandleToolsCSVImport: GET form, POST upload (preview), POST confirm with session id + column indices.
func (a *App) HandleToolsCSVImport(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		d := map[string]any{}
		if e := strings.TrimSpace(r.URL.Query().Get("error")); e != "" {
			d["CSVError"] = e
		}
		a.renderTemplate(w, r, "tools_csv_import", a.mergeData(r, d))
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Redirect(w, r, "/tools/csv-import?error=form", http.StatusFound)
		return
	}
	action := strings.TrimSpace(r.FormValue("action"))
	if action == "confirm" {
		a.handleCSVImportConfirm(w, r, userID)
		return
	}
	file, _, err := r.FormFile("csvfile")
	if err != nil {
		http.Redirect(w, r, "/tools/csv-import?error=file", http.StatusFound)
		return
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, csvImportMaxBytes+1))
	if err != nil || len(raw) > csvImportMaxBytes {
		http.Redirect(w, r, "/tools/csv-import?error=size", http.StatusFound)
		return
	}
	_, _ = a.DB.Exec(`DELETE FROM csv_import_sessions WHERE user_id = ?`, userID)
	_, _ = a.DB.Exec(`DELETE FROM csv_import_sessions WHERE datetime(created_at) < datetime('now', '-24 hours')`)
	sid := randomHexID()
	if _, err := a.DB.Exec(
		`INSERT INTO csv_import_sessions (id, user_id, raw_csv) VALUES (?, ?, ?)`,
		sid, userID, string(raw),
	); err != nil {
		http.Redirect(w, r, "/tools/csv-import?error=db", http.StatusFound)
		return
	}
	reader := csv.NewReader(strings.NewReader(string(raw)))
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	allRows, err := reader.ReadAll()
	if err != nil || len(allRows) == 0 {
		_, _ = a.DB.Exec(`DELETE FROM csv_import_sessions WHERE id = ?`, sid)
		http.Redirect(w, r, "/tools/csv-import?error=parse", http.StatusFound)
		return
	}
	if len(allRows) > csvImportMaxRows {
		_, _ = a.DB.Exec(`DELETE FROM csv_import_sessions WHERE id = ?`, sid)
		http.Redirect(w, r, "/tools/csv-import?error=rows", http.StatusFound)
		return
	}
	headers := allRows[0]
	preview := allRows
	if len(preview) > 16 {
		preview = preview[:16]
	}
	a.renderTemplate(w, r, "tools_csv_import", a.mergeData(r, map[string]any{
		"CSVSessionID": sid,
		"CSVHeaders":   headers,
		"CSVPreview":   preview,
		"CSVNumRows":   len(allRows) - 1,
	}))
}

func (a *App) handleCSVImportConfirm(w http.ResponseWriter, r *http.Request, userID int) {
	sid := strings.TrimSpace(r.FormValue("session_id"))
	if sid == "" {
		http.Redirect(w, r, "/tools/csv-import?error=session", http.StatusFound)
		return
	}
	var raw string
	err := a.DB.QueryRow(
		`SELECT raw_csv FROM csv_import_sessions WHERE id = ? AND user_id = ?`,
		sid, userID,
	).Scan(&raw)
	if err != nil {
		http.Redirect(w, r, "/tools/csv-import?error=session", http.StatusFound)
		return
	}
	titleCol, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("col_title")))
	chCol := -1
	if v := strings.TrimSpace(r.FormValue("col_chapter")); v != "" {
		chCol, _ = strconv.Atoi(v)
	}
	stCol := -1
	if v := strings.TrimSpace(r.FormValue("col_status")); v != "" {
		stCol, _ = strconv.Atoi(v)
	}
	rtCol := -1
	if v := strings.TrimSpace(r.FormValue("col_reading_type")); v != "" {
		rtCol, _ = strconv.Atoi(v)
	}
	reader := csv.NewReader(strings.NewReader(raw))
	reader.LazyQuotes = true
	rows, err := reader.ReadAll()
	if err != nil || len(rows) < 2 {
		http.Redirect(w, r, "/tools/csv-import?error=parse", http.StatusFound)
		return
	}
	dataRows := rows[1:]
	imported := 0
	var firstErr string
	for _, row := range dataRows {
		if titleCol < 0 || titleCol >= len(row) {
			firstErr = "bad_title_col"
			break
		}
		title := sanitizeTitle(strings.TrimSpace(row[titleCol]))
		if title == "" {
			continue
		}
		ch := 0
		if chCol >= 0 && chCol < len(row) {
			ch, _ = strconv.Atoi(strings.TrimSpace(row[chCol]))
			ch = clampChapter(ch)
		}
		status := ""
		if stCol >= 0 && stCol < len(row) {
			status = normalizeStatusForWrite(strings.TrimSpace(row[stCol]))
		}
		if status == "" {
			status = normalizeStatusForWrite("En cours")
		}
		rt := ""
		if rtCol >= 0 && rtCol < len(row) {
			rt = normalizeReadingTypeForWrite(strings.TrimSpace(row[rtCol]))
		}
		if rt == "" {
			rt = normalizeReadingTypeForWrite("Autre")
		}
		_, err := a.DB.Exec(
			`INSERT INTO works (title, chapter, status, reading_type, rating, notes, user_id, updated_at)
			 VALUES (?, ?, ?, ?, 0, NULL, ?, CURRENT_TIMESTAMP)`,
			title, ch, status, rt, userID,
		)
		if err != nil {
			if firstErr == "" {
				firstErr = "insert"
			}
			continue
		}
		imported++
	}
	_, _ = a.DB.Exec(`DELETE FROM csv_import_sessions WHERE id = ?`, sid)
	if firstErr != "" && imported == 0 {
		http.Redirect(w, r, "/tools/csv-import?error="+firstErr, http.StatusFound)
		return
	}
	http.Redirect(w, r, "/tools?csv_imported="+strconv.Itoa(imported), http.StatusFound)
}

// HandleAdminEnrich shows batch catalog enrichment (admin).
func (a *App) HandleAdminEnrich(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rows, err := a.DB.Query(
		`SELECT id, title, COALESCE(reading_type, '') FROM works WHERE catalog_id IS NULL ORDER BY id ASC LIMIT 200`,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()
	type row struct {
		ID          int
		Title       string
		ReadingType string
	}
	var list []row
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.Title, &x.ReadingType); err != nil {
			continue
		}
		list = append(list, x)
	}
	last := r.URL.Query().Get("last")
	a.renderTemplate(w, r, "admin_enrich", a.mergeData(r, map[string]any{
		"EnrichQueue": list,
		"EnrichLast":  last,
	}))
}

// HandleAPIAdminEnrichRun POST JSON { "limit": 10 } — AniList only, conservative match.
func (a *App) HandleAPIAdminEnrichRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Limit int `json:"limit"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Limit <= 0 || req.Limit > 30 {
		req.Limit = 10
	}
	rows, err := a.DB.Query(
		`SELECT id, title, COALESCE(reading_type, '') FROM works WHERE catalog_id IS NULL ORDER BY id ASC LIMIT ?`,
		req.Limit,
	)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer func() { _ = rows.Close() }()

	processed := 0
	linked := 0
	skipped := 0
	var errs []string

	for rows.Next() {
		var wid int
		var title, readingType string
		if err := rows.Scan(&wid, &title, &readingType); err != nil {
			continue
		}
		processed++
		if err := a.enrichOneWorkAnilist(wid, title, readingType); err != nil {
			if len(errs) < 5 {
				errs = append(errs, fmt.Sprintf("%d:%v", wid, err))
			}
			skipped++
			time.Sleep(400 * time.Millisecond)
			continue
		}
		linked++
		time.Sleep(400 * time.Millisecond)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"processed": processed,
		"linked":    linked,
		"skipped":   skipped,
		"errors":    errs,
	})
}

func normalizeCatalogTitle(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

func (a *App) enrichOneWorkAnilist(workID int, title, readingType string) error {
	results, err := catalog.SearchAnilist(title, 8)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return errors.New("no_results")
	}
	want := normalizeCatalogTitle(title)
	var pick *catalog.AnilistResult
	for i := range results {
		if normalizeCatalogTitle(results[i].Title) == want {
			pick = &results[i]
			break
		}
	}
	if pick == nil && len(results) == 1 {
		pick = &results[0]
	}
	if pick == nil {
		return errors.New("no_confident_match")
	}
	externalID := strconv.Itoa(pick.ID)
	rt := readingType
	if strings.TrimSpace(rt) == "" {
		rt = pick.ReadingType
	}
	var catalogID int64
	err = a.DB.QueryRow(
		`SELECT id FROM catalog WHERE source = 'anilist' AND external_id = ? LIMIT 1`,
		externalID,
	).Scan(&catalogID)
	if err != nil {
		res, errIns := a.DB.Exec(
			`INSERT INTO catalog (title, reading_type, image_url, source, external_id) VALUES (?, ?, ?, 'anilist', ?)`,
			pick.Title, rt, pick.ImageURL, externalID,
		)
		if errIns != nil {
			return errIns
		}
		catalogID, _ = res.LastInsertId()
	}
	_, err = a.DB.Exec(`UPDATE works SET catalog_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND catalog_id IS NULL`, catalogID, workID)
	return err
}
