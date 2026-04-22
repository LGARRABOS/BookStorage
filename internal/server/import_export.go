package server

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ExportFormatVersion is bumped when the JSON export shape changes incompatibly.
const ExportFormatVersion = 1

const maxNotesRunes = 20000
const maxImportReportURLLen = 1800

// aniImport* types parse AniList "Export as JSON" payloads (lists or flat entry arrays).
type aniImportTitle struct {
	Romaji  string `json:"romaji"`
	English string `json:"english"`
	Native  string `json:"native"`
}
type aniImportCover struct {
	Large string `json:"large"`
}
type aniImportMedia struct {
	ID         int            `json:"id"`
	Title      aniImportTitle `json:"title"`
	Format     string         `json:"format"`
	IsAdult    bool           `json:"isAdult"`
	CoverImage aniImportCover `json:"coverImage"`
}
type aniImportEntry struct {
	Status   string         `json:"status"`
	Progress int            `json:"progress"`
	Score    float64        `json:"score"`
	Notes    string         `json:"notes"`
	Media    aniImportMedia `json:"media"`
}

func exportWorkFromAniImportEntry(e aniImportEntry) (exportWork, bool) {
	title := strings.TrimSpace(e.Media.Title.Romaji)
	if title == "" {
		title = strings.TrimSpace(e.Media.Title.English)
	}
	if title == "" {
		title = strings.TrimSpace(e.Media.Title.Native)
	}
	if title == "" {
		return exportWork{}, false
	}
	link := ""
	if e.Media.ID > 0 {
		link = "https://anilist.co/manga/" + strconv.Itoa(e.Media.ID)
	}
	return exportWork{
		Title:       title,
		Chapter:     clampChapter(e.Progress),
		Link:        link,
		Status:      normalizeStatusForWrite(mapAniListStatus(e.Status)),
		ReadingType: normalizeReadingTypeForWrite(mapAniListFormat(e.Media.Format)),
		Rating:      clampRating(int(e.Score)),
		Notes:       strings.TrimSpace(e.Notes),
		IsAdult:     e.Media.IsAdult,
		ImagePath:   strings.TrimSpace(e.Media.CoverImage.Large),
	}, true
}

// exportWork is the portable shape for JSON export/import and CSV extended columns.
// JSON export always emits every key (empty strings / null catalog_id when absent) for a stable object shape.
type exportWork struct {
	Title       string `json:"title"`
	Chapter     int    `json:"chapter"`
	Link        string `json:"link"`
	Status      string `json:"status"`
	ReadingType string `json:"reading_type"`
	Rating      int    `json:"rating"`
	Notes       string `json:"notes"`
	UpdatedAt   string `json:"updated_at"`
	CatalogID   *int   `json:"catalog_id"`
	IsAdult     bool   `json:"is_adult"`
	ImagePath   string `json:"image_path"`
}

// DuplicateMode controls import when a work with the same title already exists.
type DuplicateMode string

const (
	DuplicateSkip   DuplicateMode = "skip"
	DuplicateUpdate DuplicateMode = "update"
)

// ImportLineError records a validation problem for a source line.
type ImportLineError struct {
	Line int    `json:"line"`
	Msg  string `json:"msg"`
}

// ImportReport summarizes an import run (CSV, JSON, or file upload).
type ImportReport struct {
	Imported         int               `json:"imported"`
	SkippedDuplicate int               `json:"skipped_duplicate"`
	SkippedInvalid   int               `json:"skipped_invalid"`
	Updated          int               `json:"updated"`
	Errors           []ImportLineError `json:"errors,omitempty"`
}

func truncateNotes(s string) string {
	if utf8.RuneCountInString(s) <= maxNotesRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxNotesRunes])
}

var statusENtoFR = map[string]string{
	"reading":           "En cours",
	"completed":         "Terminé",
	"on hold":           "En pause",
	"dropped":           "Abandonné",
	"plan to read":      "À lire",
	"plan_to_read":      "À lire",
	"paused":            "En pause",
	"dropped/abandoned": "Abandonné",
}

func normalizeStatus(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "En cours"
	}
	lower := strings.ToLower(s)
	if fr, ok := statusENtoFR[lower]; ok {
		return fr
	}
	for _, st := range readingStatuses {
		if strings.EqualFold(s, st) {
			return st
		}
	}
	return s
}

func isValidStatus(s string) bool {
	for _, st := range readingStatuses {
		if s == st {
			return true
		}
	}
	return false
}

var readingTypeAliases = map[string]string{
	"comic":         "BD",
	"graphic novel": "Roman",
	"graphic_novel": "Roman",
	"ln":            "Light Novel",
	"novel":         "Roman",
}

func normalizeReadingType(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "Roman"
	}
	lower := strings.ToLower(s)
	if v, ok := readingTypeAliases[lower]; ok {
		s = v
	}
	for _, rt := range readingTypes {
		if strings.EqualFold(s, rt) {
			return rt
		}
	}
	return s
}

func isValidReadingType(s string) bool {
	for _, rt := range readingTypes {
		if s == rt {
			return true
		}
	}
	return false
}

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
	imagePath := strings.TrimSpace(w.ImagePath)
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

	if existsID > 0 {
		if mode == DuplicateSkip {
			report.SkippedDuplicate++
			return
		}
		_, err := a.DB.Exec(
			`UPDATE works SET chapter = ?, link = ?, status = ?, reading_type = ?, rating = ?, notes = ?, updated_at = CURRENT_TIMESTAMP,
			 catalog_id = ?, is_adult = ?, image_path = COALESCE(NULLIF(?, ''), image_path)
			 WHERE id = ? AND user_id = ?`,
			chapter, link, status, rtype, rating, notes,
			catID, isAdult, imagePath,
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
		`INSERT INTO works (title, chapter, link, status, reading_type, rating, notes, user_id, updated_at, catalog_id, is_adult, image_path, notify_new_chapters)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?, ?, 1)`,
		title, chapter, link, status, rtype, rating, notes, userID,
		catID, isAdult, imagePath,
	)
	if err != nil {
		report.SkippedInvalid++
		appendImportError(report, lineNum, "db_insert")
		return
	}
	report.Imported++
}

func appendImportError(report *ImportReport, line int, code string) {
	if len(report.Errors) >= 30 {
		return
	}
	report.Errors = append(report.Errors, ImportLineError{Line: line, Msg: code})
}

func redirectWithImportReport(w http.ResponseWriter, r *http.Request, rep ImportReport) {
	for len(mustJSON(rep)) > maxImportReportURLLen && len(rep.Errors) > 3 {
		rep.Errors = rep.Errors[:len(rep.Errors)-1]
	}
	b, err := json.Marshal(rep)
	if err != nil {
		http.Redirect(w, r, "/dashboard?error=import", http.StatusFound)
		return
	}
	enc := base64.RawURLEncoding.EncodeToString(b)
	base := "/dashboard"
	if ref := strings.TrimSpace(r.Referer()); ref != "" {
		if ru, err := url.Parse(ref); err == nil && ru.Path == "/tools" {
			base = "/tools"
		}
	}
	u := base + "?" + url.Values{"import_report": {enc}}.Encode()
	http.Redirect(w, r, u, http.StatusFound)
}

func mustJSON(rep ImportReport) []byte {
	b, err := json.Marshal(rep)
	if err != nil {
		return nil
	}
	return b
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

func parseExternalCSVRecords(records [][]string) ([]exportWork, bool) {
	if len(records) < 2 || len(records[0]) == 0 {
		return nil, false
	}
	headers := make([]string, len(records[0]))
	for i := range records[0] {
		headers[i] = normalizeHeader(records[0][i])
	}
	if isMALHeader(headers) {
		return parseMALCSVRecords(records, headers), true
	}
	if isAniListCSVHeader(headers) {
		return parseAniListCSVRecords(records, headers), true
	}
	return nil, false
}

func normalizeHeader(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

func headerIndex(headers []string, keys ...string) int {
	for i := range headers {
		for _, k := range keys {
			if headers[i] == k {
				return i
			}
		}
	}
	return -1
}

func safeCell(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func isMALHeader(headers []string) bool {
	return headerIndex(headers, "series_title") >= 0 &&
		(headerIndex(headers, "my_status") >= 0 || headerIndex(headers, "my_read_chapters") >= 0)
}

func isAniListCSVHeader(headers []string) bool {
	return headerIndex(headers, "anilist_id", "media_id") >= 0 &&
		headerIndex(headers, "title", "media_title") >= 0
}

func parseMALCSVRecords(records [][]string, headers []string) []exportWork {
	idxTitle := headerIndex(headers, "series_title")
	idxStatus := headerIndex(headers, "my_status")
	idxProgress := headerIndex(headers, "my_read_chapters", "my_chapters_read")
	idxScore := headerIndex(headers, "my_score")
	idxType := headerIndex(headers, "series_type")

	var out []exportWork
	for i := 1; i < len(records); i++ {
		row := records[i]
		title := safeCell(row, idxTitle)
		if title == "" {
			continue
		}
		ch, _ := strconv.Atoi(safeCell(row, idxProgress))
		rating, _ := strconv.Atoi(safeCell(row, idxScore))
		out = append(out, exportWork{
			Title:       title,
			Chapter:     clampChapter(ch),
			Status:      normalizeStatusForWrite(mapMALStatus(safeCell(row, idxStatus))),
			ReadingType: normalizeReadingTypeForWrite(mapMALType(safeCell(row, idxType))),
			Rating:      clampRating(rating),
		})
	}
	return out
}

func parseAniListCSVRecords(records [][]string, headers []string) []exportWork {
	idxTitle := headerIndex(headers, "title", "media_title")
	idxStatus := headerIndex(headers, "status")
	idxProgress := headerIndex(headers, "progress", "chapters_read")
	idxScore := headerIndex(headers, "score")
	idxType := headerIndex(headers, "format", "type")
	idxID := headerIndex(headers, "anilist_id", "media_id")

	var out []exportWork
	for i := 1; i < len(records); i++ {
		row := records[i]
		title := safeCell(row, idxTitle)
		if title == "" {
			continue
		}
		ch, _ := strconv.Atoi(safeCell(row, idxProgress))
		rating, _ := strconv.Atoi(safeCell(row, idxScore))
		aid := safeCell(row, idxID)
		link := ""
		if aid != "" {
			link = "https://anilist.co/manga/" + aid
		}
		out = append(out, exportWork{
			Title:       title,
			Chapter:     clampChapter(ch),
			Link:        link,
			Status:      normalizeStatusForWrite(mapAniListStatus(safeCell(row, idxStatus))),
			ReadingType: normalizeReadingTypeForWrite(mapAniListFormat(safeCell(row, idxType))),
			Rating:      clampRating(rating),
		})
	}
	return out
}

func mapMALStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "reading":
		return "En cours"
	case "completed":
		return "Terminé"
	case "on_hold", "on-hold":
		return "En pause"
	case "dropped":
		return "Abandonné"
	case "plan_to_read", "plan to read":
		return "À lire"
	default:
		return s
	}
}

func mapMALType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "manga", "manhwa", "manhua":
		return "Manga"
	case "novel":
		return "Roman"
	default:
		return s
	}
}

func mapAniListStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "current":
		return "En cours"
	case "completed":
		return "Terminé"
	case "paused":
		return "En pause"
	case "dropped":
		return "Abandonné"
	case "planning":
		return "À lire"
	default:
		return s
	}
}

func mapAniListFormat(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "manga", "one_shot":
		return "Manga"
	case "novel":
		return "Roman"
	default:
		return s
	}
}

func parseCSVWorkRow(record []string) (exportWork, bool) {
	if len(record) < 1 || strings.TrimSpace(record[0]) == "" {
		return exportWork{}, false
	}
	w := exportWork{
		Title:       strings.TrimSpace(record[0]),
		Status:      "En cours",
		ReadingType: "Roman",
	}
	if len(record) > 1 {
		w.Chapter, _ = strconv.Atoi(strings.TrimSpace(record[1]))
	}
	if len(record) > 2 {
		w.Link = strings.TrimSpace(record[2])
	}
	if len(record) > 3 && strings.TrimSpace(record[3]) != "" {
		w.Status = strings.TrimSpace(record[3])
	}
	if len(record) > 4 && strings.TrimSpace(record[4]) != "" {
		w.ReadingType = strings.TrimSpace(record[4])
	}
	if len(record) > 5 {
		w.Rating, _ = strconv.Atoi(strings.TrimSpace(record[5]))
		if w.Rating < 0 || w.Rating > 5 {
			w.Rating = 0
		}
	}
	if len(record) > 6 {
		w.Notes = strings.TrimSpace(record[6])
	}
	if len(record) > 7 && strings.TrimSpace(record[7]) != "" {
		if id, err := strconv.Atoi(strings.TrimSpace(record[7])); err == nil && id > 0 {
			w.CatalogID = &id
		}
	}
	if len(record) > 8 {
		switch strings.ToLower(strings.TrimSpace(record[8])) {
		case "1", "true", "yes", "oui":
			w.IsAdult = true
		}
	}
	if len(record) > 9 {
		w.ImagePath = strings.TrimSpace(record[9])
	}
	return w, true
}

func parseDuplicateMode(s string) DuplicateMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(DuplicateUpdate), "merge", "overwrite":
		return DuplicateUpdate
	default:
		return DuplicateSkip
	}
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
				http.Redirect(w, r, "/dashboard?error=import", http.StatusFound)
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

func parseAniListExportJSON(data []byte) ([]exportWork, bool) {
	type aniList struct {
		Entries []aniImportEntry `json:"entries"`
	}
	type aniRoot struct {
		Lists []aniList `json:"lists"`
	}
	var root aniRoot
	if err := json.Unmarshal(data, &root); err == nil && len(root.Lists) > 0 {
		var out []exportWork
		for _, l := range root.Lists {
			for _, e := range l.Entries {
				if w, ok := exportWorkFromAniImportEntry(e); ok {
					out = append(out, w)
				}
			}
		}
		return out, len(out) > 0
	}

	var entries []aniImportEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, false
	}
	var out []exportWork
	for _, e := range entries {
		if w, ok := exportWorkFromAniImportEntry(e); ok {
			out = append(out, w)
		}
	}
	return out, len(out) > 0
}
