package server

import (
	"encoding/json"
	"strconv"
	"strings"
)

// exportWork is the portable shape for JSON export/import and CSV extended columns.
// JSON export always emits every key (empty strings / null catalog_id when absent) for a stable object shape.
type exportWork struct {
	Title         string `json:"title"`
	Chapter       int    `json:"chapter"`
	Link          string `json:"link"`
	Status        string `json:"status"`
	ReadingType   string `json:"reading_type"`
	Rating        int    `json:"rating"`
	Notes         string `json:"notes"`
	UpdatedAt     string `json:"updated_at"`
	CatalogID     *int   `json:"catalog_id"`
	IsAdult       bool   `json:"is_adult"`
	ImagePath     string `json:"image_path"`
	StartedAt     string `json:"started_at,omitempty"`
	LastChapterAt string `json:"last_chapter_at,omitempty"`
	FinishedAt    string `json:"finished_at,omitempty"`
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
	"comic":         "Manga",
	"graphic novel": "Manga",
	"graphic_novel": "Manga",
	"ln":            "Light Novel",
	"lightnovel":    "Light Novel",
	"light_novel":   "Light Novel",
	"novel":         "Light Novel",
	"manhwa":        "Webtoon",
	"manhua":        "Webtoon",
	"webtoon":       "Webtoon",
	"manga":         "Manga",
}

func normalizeReadingType(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return readingTypes[0]
	}
	lower := strings.ToLower(s)
	if v, ok := readingTypeAliases[lower]; ok {
		s = v
	}
	switch strings.ToLower(s) {
	case "manhwa", "manhua":
		return "Webtoon"
	case "roman", "bd", "autre", "18+":
		return "Manga"
	}
	for _, rt := range readingTypes {
		if strings.EqualFold(s, rt) {
			return rt
		}
	}
	return readingTypes[0]
}

func isValidReadingType(s string) bool {
	for _, rt := range readingTypes {
		if s == rt {
			return true
		}
	}
	return false
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
	case "manga":
		return "Manga"
	case "manhwa", "manhua":
		return "Webtoon"
	case "novel":
		return "Light Novel"
	default:
		return normalizeReadingType(s)
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
		return "Light Novel"
	default:
		return normalizeReadingType(s)
	}
}

func parseCSVWorkRow(record []string) (exportWork, bool) {
	if len(record) < 1 || strings.TrimSpace(record[0]) == "" {
		return exportWork{}, false
	}
	w := exportWork{
		Title:       strings.TrimSpace(record[0]),
		Status:      "En cours",
		ReadingType: "Manga",
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
	if len(record) > 10 {
		w.StartedAt = strings.TrimSpace(record[10])
	}
	if len(record) > 11 {
		w.LastChapterAt = strings.TrimSpace(record[11])
	}
	if len(record) > 12 {
		w.FinishedAt = strings.TrimSpace(record[12])
	}
	return w, true
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
