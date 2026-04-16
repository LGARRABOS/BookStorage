package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bookstorage/internal/catalog"
	"bookstorage/internal/database"
	"bookstorage/internal/i18n"
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
	if a.DB.B == database.BackendPostgres {
		_, _ = a.DB.Exec(`DELETE FROM csv_import_sessions WHERE created_at < NOW() - interval '24 hours'`)
	} else {
		_, _ = a.DB.Exec(`DELETE FROM csv_import_sessions WHERE datetime(created_at) < datetime('now', '-24 hours')`)
	}
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

// enrichWorkCandidate is one AniList search row for admin UI (manual pick).
type enrichWorkCandidate struct {
	AnilistID   int    `json:"anilist_id"`
	AnilistURL  string `json:"anilist_url,omitempty"`
	Title       string `json:"title"`
	ReadingType string `json:"reading_type,omitempty"`
}

// enrichWorkItem is one work outcome from a batch enrich run (or building block for APIs).
type enrichWorkItem struct {
	WorkID       int                   `json:"work_id"`
	Title        string                `json:"title"`
	ReadingType  string                `json:"reading_type,omitempty"`
	Status       string                `json:"status"` // linked, skipped, error
	Reason       string                `json:"reason,omitempty"`
	ReasonLabel  string                `json:"reason_label,omitempty"`
	Error        string                `json:"error,omitempty"`
	AnilistID    int                   `json:"anilist_id,omitempty"`
	AnilistURL   string                `json:"anilist_url,omitempty"`
	MatchedTitle string                `json:"matched_title,omitempty"`
	CatalogID    int64                 `json:"catalog_id,omitempty"`
	Candidates   []enrichWorkCandidate `json:"candidates,omitempty"`
}

func anilistMangaPublicURL(id int) string {
	return "https://anilist.co/manga/" + strconv.Itoa(id)
}

func enrichCandidatesFromResults(rs []catalog.AnilistResult, max int) []enrichWorkCandidate {
	var out []enrichWorkCandidate
	for i := range rs {
		if len(out) >= max {
			break
		}
		out = append(out, enrichWorkCandidate{
			AnilistID:   rs[i].ID,
			AnilistURL:  anilistMangaPublicURL(rs[i].ID),
			Title:       rs[i].Title,
			ReadingType: rs[i].ReadingType,
		})
	}
	return out
}

func (a *App) upsertCatalogFromAnilistResult(pick *catalog.AnilistResult, readingType string) (int64, error) {
	if pick == nil {
		return 0, errors.New("nil_pick")
	}
	externalID := strconv.Itoa(pick.ID)
	rt := readingType
	if strings.TrimSpace(rt) == "" {
		rt = pick.ReadingType
	}
	var catalogID int64
	err := a.DB.QueryRow(
		`SELECT id FROM catalog WHERE source = 'anilist' AND external_id = ? LIMIT 1`,
		externalID,
	).Scan(&catalogID)
	if err == nil {
		return catalogID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, errIns := a.DB.Exec(
		`INSERT INTO catalog (title, reading_type, image_url, source, external_id) VALUES (?, ?, ?, 'anilist', ?)`,
		pick.Title, rt, pick.ImageURL, externalID,
	)
	if errIns != nil {
		return 0, errIns
	}
	catalogID, errID := res.LastInsertId()
	if errID != nil {
		return 0, errID
	}
	return catalogID, nil
}

// HandleAdminEnrich shows batch catalog enrichment (admin).
func (a *App) HandleAdminEnrich(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	last := r.URL.Query().Get("last")
	// La file « sans catalogue » est chargée en JSON via GET /api/admin/enrich/queue (toujours à jour).
	a.renderTemplate(w, r, "admin_enrich", a.mergeData(r, map[string]any{
		"EnrichLast": last,
	}))
}

const enrichQueueListLimit = 200

// enrichAnilistEligibleSQL filtre les œuvres encore éligibles au rattachement auto AniList.
const enrichAnilistEligibleSQL = `catalog_id IS NULL AND COALESCE(anilist_enrich_opt_out, 0) = 0`

type enrichQueueWork struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	ReadingType string `json:"reading_type"`
}

// HandleAPIAdminEnrichQueue GET — œuvres sans catalog_id (file enrichissement), pour actualisation UI.
func (a *App) HandleAPIAdminEnrichQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var total int
	if err := a.DB.QueryRow(`SELECT COUNT(*) FROM works WHERE ` + enrichAnilistEligibleSQL).Scan(&total); err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "db_error")
		return
	}
	rows, err := a.DB.Query(
		`SELECT id, title, COALESCE(reading_type, '') FROM works WHERE `+enrichAnilistEligibleSQL+` ORDER BY id ASC LIMIT ?`,
		enrichQueueListLimit,
	)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "db_error")
		return
	}
	defer func() { _ = rows.Close() }()
	var works []enrichQueueWork
	for rows.Next() {
		var qw enrichQueueWork
		if err := rows.Scan(&qw.ID, &qw.Title, &qw.ReadingType); err != nil {
			a.apiWriteError(w, http.StatusInternalServerError, "db_error")
			return
		}
		works = append(works, qw)
	}
	if err := rows.Err(); err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "db_error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"works": works,
		"total": total,
	})
}

// HandleAPIAdminEnrichRun POST JSON { "limit": 10 } — AniList only, conservative match.
func (a *App) HandleAPIAdminEnrichRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Limit   int `json:"limit"`
		AfterID int `json:"after_id"` // si > 0 : ignorer les id <= after_id (lot 1 par 1 après une œuvre ignorée)
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Limit <= 0 || req.Limit > 30 {
		req.Limit = 10
	}
	// Une requête par itération (LIMIT 1) : évite de traiter des ids figés au début du handler
	// alors qu’un autre onglet / une autre requête a déjà posé catalog_id entre-temps.
	cursor := req.AfterID
	processed := 0
	linked := 0
	skipped := 0
	errorsN := 0
	var items []enrichWorkItem

	for i := 0; i < req.Limit; i++ {
		q := `SELECT id, title, COALESCE(reading_type, '') FROM works WHERE ` + enrichAnilistEligibleSQL
		args := []any{}
		if cursor > 0 {
			q += ` AND id > ?`
			args = append(args, cursor)
		}
		q += ` ORDER BY id ASC LIMIT 1`
		var wid int
		var title, readingType string
		err := a.DB.QueryRow(q, args...).Scan(&wid, &title, &readingType)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		cursor = wid
		processed++
		item := a.enrichTryWorkAnilist(wid, title, readingType)
		items = append(items, item)
		switch item.Status {
		case "linked":
			linked++
		case "skipped":
			skipped++
		case "error":
			errorsN++
		}
		time.Sleep(400 * time.Millisecond)
	}

	tr := i18n.T(a.currentLang(r))
	for i := range items {
		if items[i].Reason != "" {
			if s, ok := tr["admin.enrich.reason."+items[i].Reason]; ok {
				items[i].ReasonLabel = s
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"processed": processed,
		"linked":    linked,
		"skipped":   skipped,
		"errors":    errorsN,
		"items":     items,
	})
}

func normalizeCatalogTitle(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

// stripCatalogParentheticals removes trailing parenthetical chunks, e.g. "Title (Webtoon)" → "Title".
func stripCatalogParentheticals(s string) string {
	s = strings.TrimSpace(s)
	for {
		open := strings.LastIndex(s, "(")
		if open < 0 {
			break
		}
		rest := s[open:]
		closeRel := strings.Index(rest, ")")
		if closeRel < 0 {
			break
		}
		close := open + closeRel
		s = strings.TrimSpace(s[:open] + s[close+1:])
	}
	return strings.TrimSpace(s)
}

func anilistResultTitleRawVariants(r *catalog.AnilistResult) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, t := range []string{r.Title, r.TitleRomaji, r.TitleEnglish} {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func catalogWordRecall(a, b string) float64 {
	aw := strings.Fields(normalizeCatalogTitle(a))
	bw := strings.Fields(normalizeCatalogTitle(b))
	if len(aw) == 0 || len(bw) == 0 {
		return 0
	}
	bset := make(map[string]struct{}, len(bw))
	for _, w := range bw {
		bset[w] = struct{}{}
	}
	hit := 0
	for _, w := range aw {
		if _, ok := bset[w]; ok {
			hit++
		}
	}
	return float64(hit) / float64(len(aw))
}

func catalogTitleSimilarity(workTitle, candidateTitle string) float64 {
	a := stripCatalogParentheticals(workTitle)
	b := stripCatalogParentheticals(candidateTitle)
	na := normalizeCatalogTitle(a)
	nb := normalizeCatalogTitle(b)
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 1
	}
	return max(catalogWordRecall(a, b), catalogWordRecall(b, a))
}

// enrichPickAnilistResult picks one AniList row for a local work title (exact variant, single-result fallback, then fuzzy gap).
func enrichPickAnilistResult(title string, results []catalog.AnilistResult) *catalog.AnilistResult {
	if len(results) == 0 {
		return nil
	}
	want := normalizeCatalogTitle(stripCatalogParentheticals(title))
	for i := range results {
		for _, vt := range anilistResultTitleRawVariants(&results[i]) {
			if normalizeCatalogTitle(stripCatalogParentheticals(vt)) == want {
				return &results[i]
			}
		}
	}
	if len(results) == 1 {
		return &results[0]
	}
	const minScore = 0.68
	const minGap = 0.02 // best score must exceed runner-up by this much (avoids ambiguous multi-match).
	best := -1.0
	second := -1.0
	bestIdx := -1
	for i := range results {
		sc := 0.0
		for _, vt := range anilistResultTitleRawVariants(&results[i]) {
			if v := catalogTitleSimilarity(title, vt); v > sc {
				sc = v
			}
		}
		if sc > best+1e-9 {
			second = best
			best = sc
			bestIdx = i
		} else if sc > second+1e-9 {
			second = sc
		}
	}
	if best >= minScore && bestIdx >= 0 && (second < 0 || best >= second+minGap) {
		return &results[bestIdx]
	}
	return nil
}

func (a *App) enrichTryWorkAnilist(workID int, title, readingType string) enrichWorkItem {
	out := enrichWorkItem{WorkID: workID, Title: title, ReadingType: readingType}
	var catID sql.NullInt64
	if err := a.DB.QueryRow(`SELECT catalog_id FROM works WHERE id = ?`, workID).Scan(&catID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			out.Status = "error"
			out.Error = "work not found"
			return out
		}
		out.Status = "error"
		out.Error = err.Error()
		return out
	}
	if catID.Valid && catID.Int64 > 0 {
		out.Status = "skipped"
		out.Reason = "already_linked"
		return out
	}
	searchT := strings.TrimSpace(stripCatalogParentheticals(title))
	if searchT == "" {
		searchT = strings.TrimSpace(title)
	}
	results, err := catalog.SearchAnilist(searchT, 8)
	if err != nil {
		out.Status = "error"
		out.Error = err.Error()
		return out
	}
	if len(results) > 0 {
		out.Candidates = enrichCandidatesFromResults(results, 8)
	}
	if len(results) == 0 {
		out.Status = "skipped"
		out.Reason = "no_results"
		out.Candidates = nil
		return out
	}
	pick := enrichPickAnilistResult(title, results)
	if pick == nil {
		out.Status = "skipped"
		out.Reason = "no_confident_match"
		return out
	}
	catalogID, err := a.upsertCatalogFromAnilistResult(pick, readingType)
	if err != nil {
		out.Status = "error"
		out.Error = err.Error()
		return out
	}
	res, err := a.DB.Exec(`UPDATE works SET catalog_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND catalog_id IS NULL`, catalogID, workID)
	if err != nil {
		out.Status = "error"
		out.Error = err.Error()
		return out
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		out.Status = "skipped"
		out.Reason = "already_linked"
		out.Candidates = nil
		return out
	}
	if err := a.clearWorkUploadedImage(workID); err != nil {
		out.Status = "error"
		out.Error = err.Error()
		return out
	}
	out.Status = "linked"
	out.AnilistID = pick.ID
	out.AnilistURL = anilistMangaPublicURL(pick.ID)
	out.MatchedTitle = pick.Title
	out.CatalogID = catalogID
	return out
}

// HandleAPIAdminEnrichSearch POST JSON { "q": "...", "limit": 12 } — AniList title search (admin).
func (a *App) HandleAPIAdminEnrichSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Q     string `json:"q"`
		Limit int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	req.Q = strings.TrimSpace(req.Q)
	if req.Q == "" {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.Limit <= 0 || req.Limit > 15 {
		req.Limit = 12
	}
	results, err := catalog.SearchAnilist(req.Q, req.Limit)
	if err != nil {
		a.apiWriteError(w, http.StatusBadGateway, "anilist_error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"results": enrichCandidatesFromResults(results, req.Limit),
	})
}

// HandleAPIAdminEnrichLink POST JSON { "work_id": 1, "anilist_id": 12345 } — attach work to AniList media (admin).
func (a *App) HandleAPIAdminEnrichLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		WorkID    int `json:"work_id"`
		AnilistID int `json:"anilist_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if req.WorkID <= 0 || req.AnilistID <= 0 {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	var exists int
	if err := a.DB.QueryRow(`SELECT COUNT(*) FROM works WHERE id = ?`, req.WorkID).Scan(&exists); err != nil || exists == 0 {
		a.apiWriteError(w, http.StatusNotFound, "not_found")
		return
	}
	var readingType string
	_ = a.DB.QueryRow(`SELECT COALESCE(reading_type, '') FROM works WHERE id = ?`, req.WorkID).Scan(&readingType)

	detail, err := catalog.GetMediaByID(req.AnilistID)
	if err != nil {
		a.apiWriteError(w, http.StatusBadGateway, "anilist_error")
		return
	}
	if detail == nil {
		a.apiWriteError(w, http.StatusNotFound, "anilist_not_found")
		return
	}
	pick := catalog.AnilistMediaToResult(detail.RawMedia)
	catalogID, err := a.upsertCatalogFromAnilistResult(&pick, readingType)
	if err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "catalog_upsert_failed")
		return
	}
	if _, err := a.DB.Exec(`UPDATE works SET catalog_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, catalogID, req.WorkID); err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "update_failed")
		return
	}
	if err := a.clearWorkUploadedImage(req.WorkID); err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "clear_image_failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":            true,
		"work_id":       req.WorkID,
		"anilist_id":    pick.ID,
		"matched_title": pick.Title,
		"catalog_id":    catalogID,
	})
}

// handleAdminEnrichWorkMutation handles POST JSON { "work_id": n } with a single-placeholder UPDATE (? = work id).
func (a *App) handleAdminEnrichWorkMutation(w http.ResponseWriter, r *http.Request, query string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		WorkID int `json:"work_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if body.WorkID <= 0 {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	res, err := a.DB.Exec(query, body.WorkID)
	if err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "update_failed")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		a.apiWriteError(w, http.StatusNotFound, "not_found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "work_id": body.WorkID})
}

// HandleAPIAdminEnrichUnlink POST JSON { "work_id": 1 } — clear catalog_id (admin).
func (a *App) HandleAPIAdminEnrichUnlink(w http.ResponseWriter, r *http.Request) {
	a.handleAdminEnrichWorkMutation(w, r, `UPDATE works SET catalog_id = NULL, anilist_enrich_opt_out = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?`)
}

// HandleAPIAdminEnrichOptOut POST JSON { "work_id": 1 } — exclure l’œuvre du lot AniList (sans catalogue).
func (a *App) HandleAPIAdminEnrichOptOut(w http.ResponseWriter, r *http.Request) {
	a.handleAdminEnrichWorkMutation(w, r, `UPDATE works SET anilist_enrich_opt_out = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND catalog_id IS NULL`)
}

// HandleAPIAdminEnrichOptIn POST JSON { "work_id": 1 } — réinclure l’œuvre dans l’enrichissement AniList.
func (a *App) HandleAPIAdminEnrichOptIn(w http.ResponseWriter, r *http.Request) {
	a.handleAdminEnrichWorkMutation(w, r, `UPDATE works SET anilist_enrich_opt_out = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND catalog_id IS NULL`)
}

const enrichAdminWorksMaxLimit = 500
const enrichAdminWorksDefaultLimit = 100

// adminEnrichWorkListRow is one row for GET /api/admin/enrich/works.
type adminEnrichWorkListRow struct {
	ID                  int    `json:"id"`
	Title               string `json:"title"`
	ReadingType         string `json:"reading_type"`
	UserID              int    `json:"user_id"`
	CatalogID           *int64 `json:"catalog_id,omitempty"`
	CatalogTitle        string `json:"catalog_title,omitempty"`
	CatalogSource       string `json:"catalog_source,omitempty"`
	CatalogExternalID   string `json:"catalog_external_id,omitempty"`
	AnilistEnrichOptOut bool   `json:"anilist_enrich_opt_out,omitempty"`
}

// HandleAPIAdminEnrichWorks GET ?q=&limit=&offset= — paginated list of all works (admin).
func (a *App) HandleAPIAdminEnrichWorks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	titleQ := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := enrichAdminWorksDefaultLimit
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
			if limit > enrichAdminWorksMaxLimit {
				limit = enrichAdminWorksMaxLimit
			}
		}
	}
	offset := 0
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	where := "1=1"
	args := []any{}
	if titleQ != "" {
		where = "LOWER(w.title) LIKE ?"
		args = append(args, "%"+strings.ToLower(titleQ)+"%")
	}

	countSQL := "SELECT COUNT(*) FROM works w WHERE " + where
	var total int
	if err := a.DB.QueryRow(countSQL, args...).Scan(&total); err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "db_error")
		return
	}

	listSQL := `SELECT w.id, w.title, COALESCE(w.reading_type, ''), w.user_id, w.catalog_id,
		COALESCE(c.title, ''), COALESCE(c.source, ''), COALESCE(c.external_id, ''),
		COALESCE(w.anilist_enrich_opt_out, 0)
		FROM works w
		LEFT JOIN catalog c ON c.id = w.catalog_id
		WHERE ` + where + ` ORDER BY w.id LIMIT ? OFFSET ?`
	argsList := append(append([]any{}, args...), limit, offset)
	rows, err := a.DB.Query(listSQL, argsList...)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "db_error")
		return
	}
	defer func() { _ = rows.Close() }()

	var works []adminEnrichWorkListRow
	for rows.Next() {
		var row adminEnrichWorkListRow
		var catalogID sql.NullInt64
		var optOut int
		if err := rows.Scan(&row.ID, &row.Title, &row.ReadingType, &row.UserID, &catalogID, &row.CatalogTitle, &row.CatalogSource, &row.CatalogExternalID, &optOut); err != nil {
			a.apiWriteError(w, http.StatusInternalServerError, "db_error")
			return
		}
		if catalogID.Valid {
			v := catalogID.Int64
			row.CatalogID = &v
		}
		row.AnilistEnrichOptOut = optOut != 0
		works = append(works, row)
	}
	if err := rows.Err(); err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "db_error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"works":  works,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}
