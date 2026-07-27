package server

import (
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type exportBdWork struct {
	Title      string `json:"title"`
	Tome       int    `json:"tome"`
	TotalTomes *int   `json:"total_tomes,omitempty"`
	Link       string `json:"link,omitempty"`
	Status     string `json:"status,omitempty"`
	BdType     string `json:"bd_type,omitempty"`
	Rating     int    `json:"rating,omitempty"`
	Notes      string `json:"notes,omitempty"`
	IsAdult    bool   `json:"is_adult,omitempty"`
	ImagePath  string `json:"image_path,omitempty"`
	Source     string `json:"source,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
}

func (a *App) importOneBdWork(userID, lineNum int, w exportBdWork, mode DuplicateMode, report *ImportReport) {
	title := sanitizeTitle(w.Title)
	if title == "" {
		appendImportError(report, lineNum, "empty_title")
		return
	}
	status := normalizeBdStatusForWrite(w.Status)
	bdType := normalizeBdTypeForWrite(w.BdType)
	rating := clampRating(w.Rating)
	notes := truncateNotes(strings.TrimSpace(w.Notes))
	tome := clampChapter(w.Tome)
	link := strings.TrimSpace(w.Link)
	imagePath := sanitizeImportImagePath(w.ImagePath)
	source := strings.TrimSpace(w.Source)
	if source == "" {
		source = "manual"
	}
	externalID := strings.TrimSpace(w.ExternalID)
	isAdult := 0
	if w.IsAdult {
		isAdult = 1
	}
	var totalArg any
	if w.TotalTomes != nil && *w.TotalTomes > 0 {
		totalArg = *w.TotalTomes
	}
	var imgArg any
	if imagePath != "" {
		imgArg = imagePath
	}
	var extArg any
	if externalID != "" {
		extArg = externalID
	}
	startedAt := nullIfEmpty(strings.TrimSpace(w.StartedAt))
	finishedAt := nullIfEmpty(strings.TrimSpace(w.FinishedAt))

	var existsID int
	err := a.DB.QueryRow(
		`SELECT id FROM bd_works WHERE user_id = ? AND title = ?`,
		userID, title,
	).Scan(&existsID)
	if err != nil && err != sql.ErrNoRows {
		report.SkippedInvalid++
		appendImportError(report, lineNum, "db_lookup")
		return
	}
	if err == nil {
		if mode == DuplicateSkip {
			report.SkippedDuplicate++
			return
		}
		_, err = a.DB.Exec(
			`UPDATE bd_works SET tome = ?, total_tomes = ?, link = ?, status = ?, bd_type = ?,
             rating = ?, notes = ?, is_adult = ?, image_path = COALESCE(NULLIF(?, ''), image_path), source = ?, external_id = ?,
             started_at = COALESCE(?, started_at), finished_at = COALESCE(?, finished_at), updated_at = CURRENT_TIMESTAMP
             WHERE id = ? AND user_id = ?`,
			tome, totalArg, link, status, bdType, rating, notes, isAdult, imgArg, source, extArg,
			startedAt, finishedAt, existsID, userID,
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
		`INSERT INTO bd_works (title, tome, total_tomes, status, bd_type, link, image_path, rating, notes, is_adult, source, external_id, user_id, updated_at, started_at, finished_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?)`,
		title, tome, totalArg, status, bdType, link, imgArg, rating, notes, isAdult, source, extArg, userID, startedAt, finishedAt,
	)
	if err != nil {
		report.SkippedInvalid++
		appendImportError(report, lineNum, "db_insert")
		return
	}
	report.Imported++
}

func (a *App) redirectWithBdImportReport(w http.ResponseWriter, r *http.Request, userID int, rep ImportReport) {
	_ = userID
	for len(mustJSON(rep)) > maxImportReportURLLen && len(rep.Errors) > 3 {
		rep.Errors = rep.Errors[:len(rep.Errors)-1]
	}
	b, err := json.Marshal(rep)
	if err != nil {
		http.Redirect(w, r, pathToolsBd+"?error=import", http.StatusFound)
		return
	}
	enc := base64.RawURLEncoding.EncodeToString(b)
	base := pathToolsBd
	if ref := strings.TrimSpace(r.Referer()); ref != "" {
		if ru, err := url.Parse(ref); err == nil && ru.Path == pathBdDashboard {
			base = pathBdDashboard
		}
	}
	http.Redirect(w, r, base+"?"+url.Values{"import_report": {enc}}.Encode(), http.StatusFound)
}

func parseBdCSVRecords(records [][]string) ([]exportBdWork, bool) {
	if len(records) < 2 {
		return nil, false
	}
	headers := make([]string, len(records[0]))
	for i, h := range records[0] {
		headers[i] = strings.ToLower(strings.TrimSpace(h))
	}
	idxTitle := headerIndex(headers, "title")
	if idxTitle < 0 {
		return nil, false
	}
	idxTome := headerIndex(headers, "tome", "volume", "album")
	idxTotal := headerIndex(headers, "totaltomes", "total_tomes", "volumes")
	idxLink := headerIndex(headers, "link", "url")
	idxStatus := headerIndex(headers, "status")
	idxType := headerIndex(headers, "type", "bd_type", "bdtype")
	idxRating := headerIndex(headers, "rating", "score")
	idxNotes := headerIndex(headers, "notes", "comment")
	idxAdult := headerIndex(headers, "isadult", "is_adult")
	idxImage := headerIndex(headers, "imagepath", "image_path", "cover")
	idxSource := headerIndex(headers, "source")
	idxExt := headerIndex(headers, "externalid", "external_id")

	var out []exportBdWork
	for i := 1; i < len(records); i++ {
		row := records[i]
		title := safeCell(row, idxTitle)
		if title == "" {
			continue
		}
		tome, _ := strconv.Atoi(safeCell(row, idxTome))
		rating, _ := strconv.Atoi(safeCell(row, idxRating))
		w := exportBdWork{
			Title:      title,
			Tome:       clampChapter(tome),
			Link:       safeCell(row, idxLink),
			Status:     normalizeBdStatusForWrite(safeCell(row, idxStatus)),
			BdType:     normalizeBdTypeForWrite(safeCell(row, idxType)),
			Rating:     clampRating(rating),
			Notes:      safeCell(row, idxNotes),
			IsAdult:    safeCell(row, idxAdult) == "1" || strings.EqualFold(safeCell(row, idxAdult), "true"),
			ImagePath:  safeCell(row, idxImage),
			Source:     safeCell(row, idxSource),
			ExternalID: safeCell(row, idxExt),
		}
		if w.Source == "" {
			w.Source = "manual"
		}
		if tot, err := strconv.Atoi(safeCell(row, idxTotal)); err == nil && tot > 0 {
			w.TotalTomes = &tot
		}
		out = append(out, w)
	}
	return out, len(out) > 0
}

func (a *App) HandleBdExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)

	updatedAtExpr := `COALESCE(updated_at, '')`
	dateExpr := func(col string) string { return `COALESCE(` + col + `, '')` }
	if a.Settings != nil && a.Settings.UsePostgres() {
		updatedAtExpr = `COALESCE(to_char(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'), '')`
		dateExpr = func(col string) string {
			return `COALESCE(to_char(` + col + ` AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'), '')`
		}
	}

	rows, err := a.DB.Query(
		`SELECT title, COALESCE(tome, 0), total_tomes, COALESCE(link, ''), COALESCE(status, ''), COALESCE(bd_type, ''),
                COALESCE(rating, 0), COALESCE(notes, ''), COALESCE(is_adult, 0), COALESCE(image_path, ''),
                COALESCE(source, 'manual'), COALESCE(external_id, ''), `+updatedAtExpr+`,
                `+dateExpr("started_at")+`, `+dateExpr("finished_at")+`
         FROM bd_works WHERE user_id = ? ORDER BY title`,
		userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()

	var list []exportBdWork
	for rows.Next() {
		var (
			wRow                                                         exportBdWork
			total                                                        sql.NullInt64
			adult                                                        int
			updatedAt, startedAt, finishedAt, link, status, btype, notes string
			imagePath, source, extID                                     string
		)
		if err := rows.Scan(
			&wRow.Title, &wRow.Tome, &total, &link, &status, &btype, &wRow.Rating, &notes, &adult,
			&imagePath, &source, &extID, &updatedAt, &startedAt, &finishedAt,
		); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		wRow.Link = link
		wRow.Status = status
		wRow.BdType = btype
		wRow.Notes = notes
		wRow.IsAdult = adult == 1
		wRow.ImagePath = imagePath
		wRow.Source = source
		wRow.ExternalID = extID
		wRow.UpdatedAt = updatedAt
		wRow.StartedAt = startedAt
		wRow.FinishedAt = finishedAt
		if total.Valid {
			v := int(total.Int64)
			wRow.TotalTomes = &v
		}
		list = append(list, wRow)
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="bookstorage-bd.json"`)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"export_version": ExportFormatVersion,
			"bd_works":       list,
		})
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="bookstorage-bd.csv"`)
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	_ = cw.Write([]string{
		"Title", "Tome", "TotalTomes", "Link", "Status", "Type", "Rating", "Notes",
		"IsAdult", "ImagePath", "Source", "ExternalID", "StartedAt", "FinishedAt",
	})
	for _, row := range list {
		adult := "0"
		if row.IsAdult {
			adult = "1"
		}
		total := ""
		if row.TotalTomes != nil {
			total = strconv.Itoa(*row.TotalTomes)
		}
		_ = cw.Write([]string{
			row.Title, strconv.Itoa(row.Tome), total, row.Link, row.Status, row.BdType,
			strconv.Itoa(row.Rating), row.Notes, adult, row.ImagePath, row.Source, row.ExternalID,
			row.StartedAt, row.FinishedAt,
		})
	}
	cw.Flush()
}

func (a *App) HandleBdImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	mode := DuplicateSkip
	if strings.EqualFold(strings.TrimSpace(r.FormValue("duplicate_mode")), "update") {
		mode = DuplicateUpdate
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Redirect(w, r, pathToolsBd+"?error=import", http.StatusFound)
		return
	}
	file, _, err := r.FormFile("import_file")
	if err != nil {
		http.Redirect(w, r, pathToolsBd+"?error=import", http.StatusFound)
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, 20<<20))
	if err != nil || len(data) == 0 {
		http.Redirect(w, r, pathToolsBd+"?error=import", http.StatusFound)
		return
	}

	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var payload struct {
			ExportVersion int            `json:"export_version"`
			BdWorks       []exportBdWork `json:"bd_works"`
			Works         []exportBdWork `json:"works"`
		}
		report := ImportReport{}
		if err := json.Unmarshal(data, &payload); err != nil || (len(payload.BdWorks) == 0 && len(payload.Works) == 0) {
			var only []exportBdWork
			if err2 := json.Unmarshal(data, &only); err2 != nil || len(only) == 0 {
				http.Redirect(w, r, pathToolsBd+"?error=import", http.StatusFound)
				return
			}
			payload.BdWorks = only
		}
		rows := payload.BdWorks
		if len(rows) == 0 {
			rows = payload.Works
		}
		for i, row := range rows {
			a.importOneBdWork(userID, i+1, row, mode, &report)
		}
		a.redirectWithBdImportReport(w, r, userID, report)
		return
	}

	cr := csv.NewReader(strings.NewReader(string(data)))
	cr.Comma = ';'
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	records, err := cr.ReadAll()
	if err != nil || len(records) == 0 {
		cr2 := csv.NewReader(strings.NewReader(string(data)))
		cr2.Comma = ','
		cr2.LazyQuotes = true
		cr2.FieldsPerRecord = -1
		records, err = cr2.ReadAll()
	}
	if err != nil {
		http.Redirect(w, r, pathToolsBd+"?error=import", http.StatusFound)
		return
	}
	rows, ok := parseBdCSVRecords(records)
	if !ok {
		http.Redirect(w, r, pathToolsBd+"?error=import", http.StatusFound)
		return
	}
	report := ImportReport{}
	for i, row := range rows {
		a.importOneBdWork(userID, i+1, row, mode, &report)
	}
	a.redirectWithBdImportReport(w, r, userID, report)
}
