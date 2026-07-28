package server

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (a *App) catalogIDExists(id int64) bool {
	if id <= 0 {
		return false
	}
	var one int
	err := a.DB.QueryRow(`SELECT 1 FROM catalog WHERE id = ?`, id).Scan(&one)
	return err == nil
}

func (a *App) resolveCatalogIDField(w *exportWork) sql.NullInt64 {
	if w.CatalogID == nil || *w.CatalogID <= 0 {
		return sql.NullInt64{}
	}
	id := int64(*w.CatalogID)
	if !a.catalogIDExists(id) {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: id, Valid: true}
}

func (a *App) importOneWork(userID int, lineNum int, w exportWork, mode DuplicateMode, report *ImportReport) {
	title := strings.TrimSpace(w.Title)
	if title == "" {
		report.SkippedInvalid++
		appendImportError(report, lineNum, "empty_title")
		return
	}

	status := normalizeStatus(w.Status)
	if !isValidStatus(status) {
		report.SkippedInvalid++
		appendImportError(report, lineNum, "invalid_status")
		return
	}

	rtype := normalizeReadingType(w.ReadingType)
	if !isValidReadingType(rtype) {
		report.SkippedInvalid++
		appendImportError(report, lineNum, "invalid_type")
		return
	}

	rating := clampRating(w.Rating)

	notes := truncateNotes(strings.TrimSpace(w.Notes))
	chapter := clampChapter(w.Chapter)
	link := strings.TrimSpace(w.Link)
	imagePath := sanitizeImportImagePath(w.ImagePath)
	catID := a.resolveCatalogIDField(&w)
	isAdult := 0
	if w.IsAdult {
		isAdult = 1
	}

	var existsID int
	err := a.DB.QueryRow(
		`SELECT id FROM works WHERE user_id = ? AND title = ?`,
		userID, title,
	).Scan(&existsID)
	if err != nil && err != sql.ErrNoRows {
		report.SkippedInvalid++
		appendImportError(report, lineNum, "db_lookup")
		return
	}

	startedAt := nullIfEmpty(strings.TrimSpace(w.StartedAt))
	lastChapterAt := nullIfEmpty(strings.TrimSpace(w.LastChapterAt))
	finishedAt := nullIfEmpty(strings.TrimSpace(w.FinishedAt))

	if existsID > 0 {
		if mode == DuplicateSkip {
			report.SkippedDuplicate++
			return
		}
		_, err := a.DB.Exec(
			`UPDATE works SET chapter = ?, link = ?, status = ?, reading_type = ?, rating = ?, notes = ?, updated_at = CURRENT_TIMESTAMP,
			 catalog_id = ?, is_adult = ?, image_path = COALESCE(NULLIF(?, ''), image_path),
			 started_at = COALESCE(?, started_at), last_chapter_at = COALESCE(?, last_chapter_at), finished_at = COALESCE(?, finished_at)
			 WHERE id = ? AND user_id = ?`,
			chapter, link, status, rtype, rating, notes,
			catID, isAdult, imagePath,
			startedAt, lastChapterAt, finishedAt,
			existsID, userID,
		)
		if err != nil {
			report.SkippedInvalid++
			appendImportError(report, lineNum, "db_update")
			return
		}
		report.Updated++
		return
	}

	_, err = a.DB.Exec(
		`INSERT INTO works (title, chapter, link, status, reading_type, rating, notes, user_id, updated_at, catalog_id, is_adult, image_path, notify_new_chapters, started_at, last_chapter_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?, ?, 1, ?, ?, ?)`,
		title, chapter, link, status, rtype, rating, notes, userID,
		catID, isAdult, imagePath, startedAt, lastChapterAt, finishedAt,
	)
	if err != nil {
		report.SkippedInvalid++
		appendImportError(report, lineNum, "db_insert")
		return
	}
	report.Imported++
}

func redirectWithImportReport(w http.ResponseWriter, r *http.Request, rep ImportReport) {
	for len(mustJSON(rep)) > maxImportReportURLLen && len(rep.Errors) > 3 {
		rep.Errors = rep.Errors[:len(rep.Errors)-1]
	}
	b, err := json.Marshal(rep)
	if err != nil {
		http.Redirect(w, r, pathMangaDashboard+"?error=import", http.StatusFound)
		return
	}
	enc := base64.RawURLEncoding.EncodeToString(b)
	base := pathMangaDashboard
	if ref := strings.TrimSpace(r.Referer()); ref != "" {
		if ru, err := url.Parse(ref); err == nil {
			switch ru.Path {
			case pathToolsManga, pathMangaTools:
				base = pathToolsManga
			case pathToolsAnime:
				base = pathToolsAnime
			}
		}
	}
	u := base + "?" + url.Values{"import_report": {enc}}.Encode()
	http.Redirect(w, r, u, http.StatusFound)
}

// ImportFromCSVRecords imports semicolon-separated rows (header optional).
func (a *App) ImportFromCSVRecords(w http.ResponseWriter, r *http.Request, userID int, records [][]string, mode DuplicateMode) {
	if externalRows, ok := parseExternalCSVRecords(records); ok {
		report := ImportReport{}
		for i, row := range externalRows {
			a.importOneWork(userID, i+1, row, mode, &report)
		}
		redirectWithImportReport(w, r, report)
		return
	}

	startIdx := 0
	if len(records) > 0 && strings.EqualFold(strings.TrimSpace(records[0][0]), "title") {
		startIdx = 1
	}
	report := ImportReport{}
	for i := startIdx; i < len(records); i++ {
		record := records[i]
		lineNum := i + 1
		w, ok := parseCSVWorkRow(record)
		if !ok {
			continue
		}
		a.importOneWork(userID, lineNum, w, mode, &report)
	}
	redirectWithImportReport(w, r, report)
}

// ImportFromJSONBytes parses a JSON export and applies it to the user's library.
func (a *App) ImportFromJSONBytes(w http.ResponseWriter, r *http.Request, userID int, data []byte, mode DuplicateMode) {
	var payload struct {
		ExportVersion int          `json:"export_version"`
		Works         []exportWork `json:"works"`
	}
	report := ImportReport{}
	if err := json.Unmarshal(data, &payload); err != nil || len(payload.Works) == 0 {
		var only []exportWork
		if err2 := json.Unmarshal(data, &only); err2 != nil || len(only) == 0 {
			if ext, ok := parseAniListExportJSON(data); ok && len(ext) > 0 {
				payload.Works = ext
			} else {
				http.Redirect(w, r, pathMangaDashboard+"?error=import", http.StatusFound)
				return
			}
		} else {
			payload.Works = only
		}
	}
	for i, row := range payload.Works {
		a.importOneWork(userID, i+1, row, mode, &report)
	}
	redirectWithImportReport(w, r, report)
}

func (a *App) HandleExport(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)

	updatedAtExpr := `COALESCE(updated_at, '')`
	dateExpr := func(col string) string { return `COALESCE(` + col + `, '')` }
	if a.Settings.UsePostgres() {
		updatedAtExpr = `COALESCE(to_char(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'), '')`
		dateExpr = func(col string) string {
			return `COALESCE(to_char(` + col + ` AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'), '')`
		}
	}
	rows, err := a.DB.Query(
		`SELECT title, chapter, link, status, reading_type, COALESCE(rating, 0), notes, `+updatedAtExpr+`,
                catalog_id, COALESCE(is_adult, 0), COALESCE(image_path, ''),
                `+dateExpr("started_at")+`, `+dateExpr("last_chapter_at")+`, `+dateExpr("finished_at")+`
         FROM works WHERE user_id = ? ORDER BY title`,
		userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()

	var works []exportWork
	for rows.Next() {
		var w exportWork
		var link, status, readingType, notes, imagePath sql.NullString
		var catalogID sql.NullInt64
		var isAdult int
		if err := rows.Scan(&w.Title, &w.Chapter, &link, &status, &readingType, &w.Rating, &notes, &w.UpdatedAt, &catalogID, &isAdult, &imagePath, &w.StartedAt, &w.LastChapterAt, &w.FinishedAt); err != nil {
			continue
		}
		if link.Valid {
			w.Link = link.String
		}
		if status.Valid {
			w.Status = status.String
		}
		if readingType.Valid {
			w.ReadingType = readingType.String
		}
		if notes.Valid {
			w.Notes = notes.String
		}
		if catalogID.Valid && catalogID.Int64 > 0 {
			cid := int(catalogID.Int64)
			w.CatalogID = &cid
		}
		w.IsAdult = isAdult != 0
		if imagePath.Valid {
			w.ImagePath = imagePath.String
		}
		works = append(works, w)
	}

	dateStr := time.Now().Format("2006-01-02")
	if r.URL.Query().Get("format") == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"bookstorage_export_%s.json\"", dateStr))
		payload := map[string]any{
			"export_version": ExportFormatVersion,
			"works":          works,
			"exported_at":    time.Now().Format(time.RFC3339),
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(payload)
		return
	}

	// CSV
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"bookstorage_export_%s.csv\"", dateStr))
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	writer.Comma = ';'
	defer writer.Flush()
	_ = writer.Write([]string{"Title", "Chapter", "Link", "Status", "Type", "Rating", "Notes", "CatalogID", "IsAdult", "ImagePath", "StartedAt", "LastChapterAt", "FinishedAt"})
	for _, row := range works {
		cat := ""
		if row.CatalogID != nil {
			cat = strconv.Itoa(*row.CatalogID)
		}
		adult := "0"
		if row.IsAdult {
			adult = "1"
		}
		_ = writer.Write([]string{
			csvSafeCell(row.Title),
			strconv.Itoa(row.Chapter),
			csvSafeCell(row.Link),
			csvSafeCell(row.Status),
			csvSafeCell(row.ReadingType),
			strconv.Itoa(row.Rating),
			csvSafeCell(row.Notes),
			cat,
			adult,
			csvSafeCell(row.ImagePath),
			csvSafeCell(row.StartedAt),
			csvSafeCell(row.LastChapterAt),
			csvSafeCell(row.FinishedAt),
		})
	}
}

// HandleImport accepts CSV or JSON (file upload or JSON body).
func (a *App) HandleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	userID, _ := a.currentUserID(r)
	mode := parseDuplicateMode(r.URL.Query().Get("duplicate_mode"))

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
		if err != nil {
			http.Redirect(w, r, pathMangaDashboard+"?error=import", http.StatusFound)
			return
		}
		a.ImportFromJSONBytes(w, r, userID, body, mode)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Redirect(w, r, pathMangaDashboard+"?error=import", http.StatusFound)
		return
	}
	mode = parseDuplicateMode(r.FormValue("duplicate_mode"))

	var file multipart.File
	var filename string
	if f, h, err := r.FormFile("import_file"); err == nil {
		file, filename = f, h.Filename
	} else if f, h, err := r.FormFile("csv_file"); err == nil {
		file, filename = f, h.Filename
	} else if f, h, err := r.FormFile("json_file"); err == nil {
		file, filename = f, h.Filename
	} else {
		http.Redirect(w, r, pathMangaDashboard+"?error=import", http.StatusFound)
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		http.Redirect(w, r, pathMangaDashboard+"?error=import", http.StatusFound)
		return
	}
	trim := strings.TrimSpace(string(data))
	isJSON := strings.HasSuffix(strings.ToLower(filename), ".json") ||
		strings.HasPrefix(trim, "{") || strings.HasPrefix(trim, "[")
	if isJSON {
		a.ImportFromJSONBytes(w, r, userID, data, mode)
		return
	}

	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = ';'
	reader.LazyQuotes = true
	records, err := reader.ReadAll()
	if err != nil {
		http.Redirect(w, r, pathMangaDashboard+"?error=import", http.StatusFound)
		return
	}
	a.ImportFromCSVRecords(w, r, userID, records, mode)
}
