package server

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// exportAnimeWork is the portable shape for anime JSON/CSV export/import.
type exportAnimeWork struct {
	Title         string `json:"title"`
	Episode       int    `json:"episode"`
	TotalEpisodes *int   `json:"total_episodes,omitempty"`
	Link          string `json:"link"`
	Status        string `json:"status"`
	AnimeType     string `json:"anime_type"`
	Rating        int    `json:"rating"`
	Notes         string `json:"notes"`
	IsAdult       bool   `json:"is_adult"`
	ImagePath     string `json:"image_path"`
	Source        string `json:"source,omitempty"`
	ExternalID    string `json:"external_id,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	FinishedAt    string `json:"finished_at,omitempty"`
}

func (a *App) redirectWithAnimeImportReport(w http.ResponseWriter, r *http.Request, userID int, rep ImportReport) {
	// Enrich covers even on re-import (duplicates skipped) when entries still lack images.
	if rep.Imported > 0 || rep.Updated > 0 || rep.SkippedDuplicate > 0 {
		a.scheduleAnimeCoverEnrichment(userID)
	}
	for len(mustJSON(rep)) > maxImportReportURLLen && len(rep.Errors) > 3 {
		rep.Errors = rep.Errors[:len(rep.Errors)-1]
	}
	b, err := json.Marshal(rep)
	if err != nil {
		http.Redirect(w, r, pathToolsAnime+"?error=import", http.StatusFound)
		return
	}
	enc := base64.RawURLEncoding.EncodeToString(b)
	base := pathToolsAnime
	if ref := strings.TrimSpace(r.Referer()); ref != "" {
		if ru, err := url.Parse(ref); err == nil && ru.Path == pathAnimeDashboard {
			base = pathAnimeDashboard
		}
	}
	http.Redirect(w, r, base+"?"+url.Values{"import_report": {enc}}.Encode(), http.StatusFound)
}

func mapMALAnimeStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "watching", "1":
		return "En cours"
	case "completed", "2":
		return "Terminé"
	case "on_hold", "on-hold", "onhold", "on hold", "3":
		return "En pause"
	case "dropped", "4":
		return "Abandonné"
	case "plan_to_watch", "plan to watch", "plantowatch", "6":
		return "À voir"
	default:
		return s
	}
}

// malScoreToStars maps MAL 0–10 scores onto BookStorage 0–5 stars.
func malScoreToStars(score int) int {
	if score <= 0 {
		return 0
	}
	if score > 10 {
		score = 10
	}
	if score <= 5 {
		return clampRating(score)
	}
	return clampRating((score + 1) / 2)
}

func mapMALAnimeType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "tv", "tv short", "tv_short":
		return "TV"
	case "movie":
		return "Film"
	case "ova":
		return "OVA"
	case "ona":
		return "ONA"
	case "special", "music":
		return "Spécial"
	default:
		return normalizeAnimeTypeForWrite(s)
	}
}

func mapAniListAnimeStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "current", "watching", "repeating":
		return "En cours"
	case "completed":
		return "Terminé"
	case "paused":
		return "En pause"
	case "dropped":
		return "Abandonné"
	case "planning", "plan_to_watch":
		return "À voir"
	default:
		return s
	}
}

func mapAniListAnimeFormat(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "TV", "TV_SHORT":
		return "TV"
	case "MOVIE":
		return "Film"
	case "OVA":
		return "OVA"
	case "ONA":
		return "ONA"
	case "SPECIAL", "MUSIC":
		return "Spécial"
	default:
		return "TV"
	}
}

func isMALAnimeHeader(headers []string) bool {
	return headerIndex(headers, "series_title") >= 0 &&
		(headerIndex(headers, "my_watched_episodes") >= 0 ||
			headerIndex(headers, "my_status") >= 0)
}

func isAniListAnimeCSVHeader(headers []string) bool {
	return headerIndex(headers, "anilist_id", "media_id") >= 0 &&
		headerIndex(headers, "title", "media_title") >= 0
}

func isBookStorageAnimeHeader(headers []string) bool {
	return headerIndex(headers, "title") >= 0 &&
		headerIndex(headers, "episode") >= 0
}

func parseMALAnimeCSVRecords(records [][]string, headers []string) []exportAnimeWork {
	idxTitle := headerIndex(headers, "series_title")
	idxStatus := headerIndex(headers, "my_status")
	idxProgress := headerIndex(headers, "my_watched_episodes", "my_watched")
	idxScore := headerIndex(headers, "my_score")
	idxType := headerIndex(headers, "series_type")
	idxTotal := headerIndex(headers, "series_episodes")

	var out []exportAnimeWork
	for i := 1; i < len(records); i++ {
		row := records[i]
		title := safeCell(row, idxTitle)
		if title == "" {
			continue
		}
		ep, _ := strconv.Atoi(safeCell(row, idxProgress))
		rating, _ := strconv.Atoi(safeCell(row, idxScore))
		w := exportAnimeWork{
			Title:     title,
			Episode:   clampChapter(ep),
			Status:    normalizeAnimeStatusForWrite(mapMALAnimeStatus(safeCell(row, idxStatus))),
			AnimeType: normalizeAnimeTypeForWrite(mapMALAnimeType(safeCell(row, idxType))),
			Rating:    malScoreToStars(rating),
			Source:    "manual",
		}
		if tot, err := strconv.Atoi(safeCell(row, idxTotal)); err == nil && tot > 0 {
			w.TotalEpisodes = &tot
		}
		out = append(out, w)
	}
	return out
}

func parseAniListAnimeCSVRecords(records [][]string, headers []string) []exportAnimeWork {
	idxTitle := headerIndex(headers, "title", "media_title")
	idxStatus := headerIndex(headers, "status")
	idxProgress := headerIndex(headers, "progress", "episodes_watched")
	idxScore := headerIndex(headers, "score")
	idxType := headerIndex(headers, "format", "type")
	idxID := headerIndex(headers, "anilist_id", "media_id")
	idxTotal := headerIndex(headers, "episodes", "total_episodes")

	var out []exportAnimeWork
	for i := 1; i < len(records); i++ {
		row := records[i]
		title := safeCell(row, idxTitle)
		if title == "" {
			continue
		}
		ep, _ := strconv.Atoi(safeCell(row, idxProgress))
		rating, _ := strconv.Atoi(safeCell(row, idxScore))
		aid := safeCell(row, idxID)
		link := ""
		if aid != "" {
			link = "https://anilist.co/anime/" + aid
		}
		w := exportAnimeWork{
			Title:      title,
			Episode:    clampChapter(ep),
			Link:       link,
			Status:     normalizeAnimeStatusForWrite(mapAniListAnimeStatus(safeCell(row, idxStatus))),
			AnimeType:  normalizeAnimeTypeForWrite(mapAniListAnimeFormat(safeCell(row, idxType))),
			Rating:     clampRating(rating),
			Source:     "anilist",
			ExternalID: aid,
		}
		if tot, err := strconv.Atoi(safeCell(row, idxTotal)); err == nil && tot > 0 {
			w.TotalEpisodes = &tot
		}
		out = append(out, w)
	}
	return out
}

func parseBookStorageAnimeCSVRecords(records [][]string, headers []string) []exportAnimeWork {
	idxTitle := headerIndex(headers, "title")
	idxEpisode := headerIndex(headers, "episode")
	idxTotal := headerIndex(headers, "totalepisodes", "total_episodes")
	idxLink := headerIndex(headers, "link")
	idxStatus := headerIndex(headers, "status")
	idxType := headerIndex(headers, "type", "anime_type")
	idxRating := headerIndex(headers, "rating")
	idxNotes := headerIndex(headers, "notes")
	idxAdult := headerIndex(headers, "isadult", "is_adult")
	idxImage := headerIndex(headers, "imagepath", "image_path")
	idxSource := headerIndex(headers, "source")
	idxExt := headerIndex(headers, "externalid", "external_id")

	var out []exportAnimeWork
	for i := 1; i < len(records); i++ {
		row := records[i]
		title := safeCell(row, idxTitle)
		if title == "" {
			continue
		}
		ep, _ := strconv.Atoi(safeCell(row, idxEpisode))
		rating, _ := strconv.Atoi(safeCell(row, idxRating))
		w := exportAnimeWork{
			Title:      title,
			Episode:    clampChapter(ep),
			Link:       safeCell(row, idxLink),
			Status:     normalizeAnimeStatusForWrite(safeCell(row, idxStatus)),
			AnimeType:  normalizeAnimeTypeForWrite(safeCell(row, idxType)),
			Rating:     clampRating(rating),
			Notes:      safeCell(row, idxNotes),
			ImagePath:  safeCell(row, idxImage),
			Source:     safeCell(row, idxSource),
			ExternalID: safeCell(row, idxExt),
			IsAdult:    safeCell(row, idxAdult) == "1" || strings.EqualFold(safeCell(row, idxAdult), "true"),
		}
		if tot, err := strconv.Atoi(safeCell(row, idxTotal)); err == nil && tot > 0 {
			w.TotalEpisodes = &tot
		}
		if w.Source == "" {
			w.Source = "manual"
		}
		out = append(out, w)
	}
	return out
}

func parseExternalAnimeCSVRecords(records [][]string) ([]exportAnimeWork, bool) {
	if len(records) < 2 || len(records[0]) == 0 {
		return nil, false
	}
	headers := make([]string, len(records[0]))
	for i := range records[0] {
		headers[i] = normalizeHeader(records[0][i])
	}
	if isBookStorageAnimeHeader(headers) {
		return parseBookStorageAnimeCSVRecords(records, headers), true
	}
	if isMALAnimeHeader(headers) {
		return parseMALAnimeCSVRecords(records, headers), true
	}
	if isAniListAnimeCSVHeader(headers) {
		return parseAniListAnimeCSVRecords(records, headers), true
	}
	return nil, false
}

func exportAnimeFromAniImportEntry(e aniImportEntry) (exportAnimeWork, bool) {
	mediaType := strings.ToUpper(strings.TrimSpace(e.Media.Type))
	if mediaType != "" && mediaType != "ANIME" {
		return exportAnimeWork{}, false
	}
	// Without type, accept TV/MOVIE/OVA/ONA formats as anime-like.
	if mediaType == "" {
		f := strings.ToUpper(strings.TrimSpace(e.Media.Format))
		switch f {
		case "MANGA", "NOVEL", "ONE_SHOT", "LIGHT_NOVEL":
			return exportAnimeWork{}, false
		}
	}

	title := strings.TrimSpace(e.Media.Title.English)
	if title == "" {
		title = strings.TrimSpace(e.Media.Title.Romaji)
	}
	if title == "" {
		title = strings.TrimSpace(e.Media.Title.Native)
	}
	if title == "" {
		return exportAnimeWork{}, false
	}
	extID := ""
	link := ""
	if e.Media.ID > 0 {
		extID = strconv.Itoa(e.Media.ID)
		link = "https://anilist.co/anime/" + extID
	}
	w := exportAnimeWork{
		Title:      title,
		Episode:    clampChapter(e.Progress),
		Link:       link,
		Status:     normalizeAnimeStatusForWrite(mapAniListAnimeStatus(e.Status)),
		AnimeType:  normalizeAnimeTypeForWrite(mapAniListAnimeFormat(e.Media.Format)),
		Rating:     clampRating(int(e.Score)),
		Notes:      strings.TrimSpace(e.Notes),
		IsAdult:    e.Media.IsAdult,
		ImagePath:  strings.TrimSpace(e.Media.CoverImage.Large),
		Source:     "anilist",
		ExternalID: extID,
	}
	if e.Media.Episodes != nil && *e.Media.Episodes > 0 {
		w.TotalEpisodes = e.Media.Episodes
	}
	return w, true
}

func parseAniListAnimeExportJSON(data []byte) ([]exportAnimeWork, bool) {
	type aniList struct {
		Entries []aniImportEntry `json:"entries"`
	}
	type aniRoot struct {
		Lists []aniList `json:"lists"`
	}
	var root aniRoot
	if err := json.Unmarshal(data, &root); err == nil && len(root.Lists) > 0 {
		var out []exportAnimeWork
		for _, l := range root.Lists {
			for _, e := range l.Entries {
				if w, ok := exportAnimeFromAniImportEntry(e); ok {
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
	var out []exportAnimeWork
	for _, e := range entries {
		if w, ok := exportAnimeFromAniImportEntry(e); ok {
			out = append(out, w)
		}
	}
	return out, len(out) > 0
}

func (a *App) importOneAnimeWork(userID int, lineNum int, w exportAnimeWork, mode DuplicateMode, report *ImportReport) {
	title := strings.TrimSpace(w.Title)
	if title == "" {
		report.SkippedInvalid++
		appendImportError(report, lineNum, "empty_title")
		return
	}
	status := normalizeAnimeStatusForWrite(w.Status)
	animeType := normalizeAnimeTypeForWrite(w.AnimeType)
	rating := clampRating(w.Rating)
	notes := truncateNotes(strings.TrimSpace(w.Notes))
	episode := clampChapter(w.Episode)
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
	if w.TotalEpisodes != nil && *w.TotalEpisodes > 0 {
		totalArg = *w.TotalEpisodes
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
		`SELECT id FROM anime_works WHERE user_id = ? AND title = ?`,
		userID, title,
	).Scan(&existsID)
	if err != nil && err != sql.ErrNoRows {
		report.SkippedInvalid++
		appendImportError(report, lineNum, "db_lookup")
		return
	}
	if err == nil {
		if mode == DuplicateSkip {
			if a.fillAnimeDuplicateCoverIfMissing(userID, existsID, imagePath, source, externalID) {
				report.Updated++
				return
			}
			report.SkippedDuplicate++
			return
		}
		_, err = a.DB.Exec(
			`UPDATE anime_works SET episode = ?, total_episodes = ?, link = ?, status = ?, anime_type = ?,
             rating = ?, notes = ?, is_adult = ?, image_path = COALESCE(NULLIF(?, ''), image_path), source = ?, external_id = ?,
             started_at = COALESCE(?, started_at), finished_at = COALESCE(?, finished_at), updated_at = CURRENT_TIMESTAMP
             WHERE id = ? AND user_id = ?`,
			episode, totalArg, link, status, animeType, rating, notes, isAdult, imgArg, source, extArg,
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
		`INSERT INTO anime_works (title, episode, total_episodes, status, anime_type, link, image_path, rating, notes, is_adult, source, external_id, user_id, updated_at, started_at, finished_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?)`,
		title, episode, totalArg, status, animeType, link, imgArg, rating, notes, isAdult, source, extArg, userID, startedAt, finishedAt,
	)
	if err != nil {
		report.SkippedInvalid++
		appendImportError(report, lineNum, "db_insert")
		return
	}
	report.Imported++
}

// fillAnimeDuplicateCoverIfMissing sets a cover on a duplicate entry that has none.
// Prefer the image from the import file; otherwise merge source/external_id so
// background AniList enrichment can resolve a cover later. Returns true when image_path was set.
func (a *App) fillAnimeDuplicateCoverIfMissing(userID, workID int, imagePath, source, externalID string) bool {
	var existingImage string
	err := a.DB.QueryRow(
		`SELECT COALESCE(image_path, '') FROM anime_works WHERE id = ? AND user_id = ?`,
		workID, userID,
	).Scan(&existingImage)
	if err != nil || strings.TrimSpace(existingImage) != "" {
		return false
	}

	cover := strings.TrimSpace(imagePath)
	if cover != "" {
		res, err := a.DB.Exec(
			`UPDATE anime_works SET image_path = ?,
             source = CASE WHEN COALESCE(TRIM(source), '') IN ('', 'manual') AND ? != '' THEN ? ELSE source END,
             external_id = COALESCE(NULLIF(TRIM(COALESCE(external_id, '')), ''), NULLIF(?, '')),
             updated_at = CURRENT_TIMESTAMP
             WHERE id = ? AND user_id = ? AND (image_path IS NULL OR TRIM(image_path) = '')`,
			cover, source, source, externalID, workID, userID,
		)
		if err != nil {
			return false
		}
		n, _ := res.RowsAffected()
		return n > 0
	}

	// No cover in the import: keep/merge MAL/AniList ids for later enrichment.
	if source != "" || externalID != "" {
		_, _ = a.DB.Exec(
			`UPDATE anime_works SET
             source = CASE WHEN COALESCE(TRIM(source), '') IN ('', 'manual') AND ? != '' THEN ? ELSE source END,
             external_id = COALESCE(NULLIF(TRIM(COALESCE(external_id, '')), ''), NULLIF(?, '')),
             updated_at = CURRENT_TIMESTAMP
             WHERE id = ? AND user_id = ? AND (image_path IS NULL OR TRIM(image_path) = '')`,
			source, source, externalID, workID, userID,
		)
	}
	return false
}

func (a *App) ImportAnimeFromCSVRecords(w http.ResponseWriter, r *http.Request, userID int, records [][]string, mode DuplicateMode) {
	rows, ok := parseExternalAnimeCSVRecords(records)
	if !ok {
		// Fallback: treat as BookStorage anime without requiring Episode header (Title first col).
		report := ImportReport{}
		for i, rec := range records {
			if i == 0 && len(rec) > 0 && strings.EqualFold(strings.TrimSpace(rec[0]), "title") {
				continue
			}
			aw, valid := parseCSVAnimeWorkRow(rec)
			if !valid {
				report.SkippedInvalid++
				appendImportError(&report, i+1, "invalid_row")
				continue
			}
			a.importOneAnimeWork(userID, i+1, aw, mode, &report)
		}
		a.redirectWithAnimeImportReport(w, r, userID, report)
		return
	}
	report := ImportReport{}
	for i, row := range rows {
		a.importOneAnimeWork(userID, i+1, row, mode, &report)
	}
	a.redirectWithAnimeImportReport(w, r, userID, report)
}

func parseCSVAnimeWorkRow(record []string) (exportAnimeWork, bool) {
	if len(record) < 1 || strings.TrimSpace(record[0]) == "" {
		return exportAnimeWork{}, false
	}
	w := exportAnimeWork{
		Title:     strings.TrimSpace(record[0]),
		Status:    "À voir",
		AnimeType: "TV",
		Source:    "manual",
	}
	if len(record) > 1 {
		w.Episode, _ = strconv.Atoi(strings.TrimSpace(record[1]))
	}
	if len(record) > 2 {
		if tot, err := strconv.Atoi(strings.TrimSpace(record[2])); err == nil && tot > 0 {
			w.TotalEpisodes = &tot
		}
	}
	if len(record) > 3 {
		w.Link = strings.TrimSpace(record[3])
	}
	if len(record) > 4 && strings.TrimSpace(record[4]) != "" {
		w.Status = strings.TrimSpace(record[4])
	}
	if len(record) > 5 && strings.TrimSpace(record[5]) != "" {
		w.AnimeType = strings.TrimSpace(record[5])
	}
	if len(record) > 6 {
		w.Rating, _ = strconv.Atoi(strings.TrimSpace(record[6]))
	}
	if len(record) > 7 {
		w.Notes = strings.TrimSpace(record[7])
	}
	if len(record) > 8 {
		w.IsAdult = strings.TrimSpace(record[8]) == "1"
	}
	if len(record) > 9 {
		w.ImagePath = strings.TrimSpace(record[9])
	}
	if len(record) > 10 && strings.TrimSpace(record[10]) != "" {
		w.Source = strings.TrimSpace(record[10])
	}
	if len(record) > 11 {
		w.ExternalID = strings.TrimSpace(record[11])
	}
	return w, true
}

// malAnimeListXML matches MyAnimeList XML export (Version 1.1.0).
type malAnimeListXML struct {
	XMLName xml.Name           `xml:"myanimelist"`
	Anime   []malAnimeEntryXML `xml:"anime"`
}

type malAnimeEntryXML struct {
	SeriesAnimeDBID   int    `xml:"series_animedb_id"`
	SeriesTitle       string `xml:"series_title"`
	SeriesType        string `xml:"series_type"`
	SeriesEpisodes    int    `xml:"series_episodes"`
	MyWatchedEpisodes int    `xml:"my_watched_episodes"`
	MyStartDate       string `xml:"my_start_date"`
	MyFinishDate      string `xml:"my_finish_date"`
	MyScore           int    `xml:"my_score"`
	MyStatus          string `xml:"my_status"`
	MyComments        string `xml:"my_comments"`
	MyTags            string `xml:"my_tags"`
}

func malAnimeDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "0000-00-00" || strings.HasPrefix(s, "0000-") {
		return ""
	}
	return s
}

func parseMALAnimeXML(data []byte) ([]exportAnimeWork, bool) {
	var list malAnimeListXML
	if err := xml.Unmarshal(data, &list); err != nil || len(list.Anime) == 0 {
		return nil, false
	}
	out := make([]exportAnimeWork, 0, len(list.Anime))
	for _, e := range list.Anime {
		title := strings.TrimSpace(e.SeriesTitle)
		if title == "" {
			continue
		}
		w := exportAnimeWork{
			Title:      title,
			Episode:    clampChapter(e.MyWatchedEpisodes),
			Status:     normalizeAnimeStatusForWrite(mapMALAnimeStatus(e.MyStatus)),
			AnimeType:  normalizeAnimeTypeForWrite(mapMALAnimeType(e.SeriesType)),
			Rating:     malScoreToStars(e.MyScore),
			StartedAt:  malAnimeDate(e.MyStartDate),
			FinishedAt: malAnimeDate(e.MyFinishDate),
			Source:     "mal",
		}
		if e.SeriesEpisodes > 0 {
			tot := e.SeriesEpisodes
			w.TotalEpisodes = &tot
		}
		if e.SeriesAnimeDBID > 0 {
			id := strconv.Itoa(e.SeriesAnimeDBID)
			w.ExternalID = id
			w.Link = "https://myanimelist.net/anime/" + id
		}
		notes := strings.TrimSpace(e.MyComments)
		if tags := strings.TrimSpace(e.MyTags); tags != "" {
			if notes != "" {
				notes += "\n"
			}
			notes += tags
		}
		w.Notes = notes
		out = append(out, w)
	}
	return out, len(out) > 0
}

func (a *App) ImportAnimeFromMALXML(w http.ResponseWriter, r *http.Request, userID int, data []byte, mode DuplicateMode) {
	rows, ok := parseMALAnimeXML(data)
	if !ok {
		http.Redirect(w, r, pathToolsAnime+"?error=import", http.StatusFound)
		return
	}
	report := ImportReport{}
	for i, row := range rows {
		a.importOneAnimeWork(userID, i+1, row, mode, &report)
	}
	a.redirectWithAnimeImportReport(w, r, userID, report)
}

func (a *App) ImportAnimeFromJSONBytes(w http.ResponseWriter, r *http.Request, userID int, data []byte, mode DuplicateMode) {
	var payload struct {
		ExportVersion int               `json:"export_version"`
		AnimeWorks    []exportAnimeWork `json:"anime_works"`
		Works         []exportAnimeWork `json:"works"` // tolerate wrong key
	}
	report := ImportReport{}
	if err := json.Unmarshal(data, &payload); err != nil || (len(payload.AnimeWorks) == 0 && len(payload.Works) == 0) {
		var only []exportAnimeWork
		if err2 := json.Unmarshal(data, &only); err2 != nil || len(only) == 0 {
			if ext, ok := parseAniListAnimeExportJSON(data); ok && len(ext) > 0 {
				payload.AnimeWorks = ext
			} else {
				http.Redirect(w, r, pathToolsAnime+"?error=import", http.StatusFound)
				return
			}
		} else {
			payload.AnimeWorks = only
		}
	}
	rows := payload.AnimeWorks
	if len(rows) == 0 {
		rows = payload.Works
	}
	for i, row := range rows {
		a.importOneAnimeWork(userID, i+1, row, mode, &report)
	}
	a.redirectWithAnimeImportReport(w, r, userID, report)
}

// HandleAnimeExport downloads the user's anime_works as CSV or JSON.
func (a *App) HandleAnimeExport(w http.ResponseWriter, r *http.Request) {
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
		`SELECT title, COALESCE(episode, 0), total_episodes, COALESCE(link, ''), COALESCE(status, ''), COALESCE(anime_type, ''),
                COALESCE(rating, 0), COALESCE(notes, ''), COALESCE(is_adult, 0), COALESCE(image_path, ''),
                COALESCE(source, 'manual'), COALESCE(external_id, ''), `+updatedAtExpr+`,
                `+dateExpr("started_at")+`, `+dateExpr("finished_at")+`
         FROM anime_works WHERE user_id = ? ORDER BY title`,
		userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()

	var list []exportAnimeWork
	for rows.Next() {
		var (
			wRow                                                         exportAnimeWork
			total                                                        sql.NullInt64
			adult                                                        int
			updatedAt, startedAt, finishedAt, link, status, atype, notes string
			imagePath, source, extID                                     string
		)
		if err := rows.Scan(
			&wRow.Title, &wRow.Episode, &total, &link, &status, &atype, &wRow.Rating, &notes, &adult,
			&imagePath, &source, &extID, &updatedAt, &startedAt, &finishedAt,
		); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		wRow.Link = link
		wRow.Status = status
		wRow.AnimeType = atype
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
			wRow.TotalEpisodes = &v
		}
		list = append(list, wRow)
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="bookstorage-anime.json"`)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"export_version": ExportFormatVersion,
			"anime_works":    list,
		})
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="bookstorage-anime.csv"`)
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	_ = cw.Write([]string{
		"Title", "Episode", "TotalEpisodes", "Link", "Status", "Type", "Rating", "Notes",
		"IsAdult", "ImagePath", "Source", "ExternalID", "StartedAt", "FinishedAt",
	})
	for _, row := range list {
		adult := "0"
		if row.IsAdult {
			adult = "1"
		}
		tot := ""
		if row.TotalEpisodes != nil {
			tot = strconv.Itoa(*row.TotalEpisodes)
		}
		_ = cw.Write([]string{
			csvSafeCell(row.Title),
			strconv.Itoa(row.Episode),
			tot,
			csvSafeCell(row.Link),
			csvSafeCell(row.Status),
			csvSafeCell(row.AnimeType),
			strconv.Itoa(row.Rating),
			csvSafeCell(row.Notes),
			adult,
			csvSafeCell(row.ImagePath),
			csvSafeCell(row.Source),
			csvSafeCell(row.ExternalID),
			csvSafeCell(row.StartedAt),
			csvSafeCell(row.FinishedAt),
		})
	}
	cw.Flush()
}

// HandleAnimeImport accepts CSV, JSON, or MAL XML (file upload or JSON body) into anime_works.
func (a *App) HandleAnimeImport(w http.ResponseWriter, r *http.Request) {
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
			http.Redirect(w, r, pathToolsAnime+"?error=import", http.StatusFound)
			return
		}
		a.ImportAnimeFromJSONBytes(w, r, userID, body, mode)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Redirect(w, r, pathToolsAnime+"?error=import", http.StatusFound)
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
		http.Redirect(w, r, pathToolsAnime+"?error=import", http.StatusFound)
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		http.Redirect(w, r, pathToolsAnime+"?error=import", http.StatusFound)
		return
	}
	trim := strings.TrimSpace(string(data))
	lowerName := strings.ToLower(filename)
	isXML := strings.HasSuffix(lowerName, ".xml") ||
		strings.HasPrefix(trim, "<?xml") ||
		strings.Contains(strings.ToLower(trim[:min(len(trim), 512)]), "<myanimelist")
	if isXML {
		a.ImportAnimeFromMALXML(w, r, userID, data, mode)
		return
	}
	isJSON := strings.HasSuffix(lowerName, ".json") ||
		strings.HasPrefix(trim, "{") || strings.HasPrefix(trim, "[")
	if isJSON {
		a.ImportAnimeFromJSONBytes(w, r, userID, data, mode)
		return
	}

	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = ';'
	reader.LazyQuotes = true
	records, err := reader.ReadAll()
	if err != nil {
		http.Redirect(w, r, pathToolsAnime+"?error=import", http.StatusFound)
		return
	}
	a.ImportAnimeFromCSVRecords(w, r, userID, records, mode)
}
