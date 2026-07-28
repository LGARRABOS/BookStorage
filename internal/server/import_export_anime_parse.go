package server

import (
	"encoding/json"
	"encoding/xml"
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
	case "hentai", "erotica":
		// Keep a listable type; adult flag is set separately via malAnimeIsAdult.
		return "OVA"
	default:
		return normalizeAnimeTypeForWrite(s)
	}
}

func malAnimeIsAdult(seriesType, tags string) bool {
	t := strings.ToLower(strings.TrimSpace(seriesType))
	if t == "hentai" || t == "erotica" || t == "rx" {
		return true
	}
	tagBlob := strings.ToLower(tags)
	for _, needle := range []string{"hentai", "erotica", "nsfw", "18+", "+18"} {
		if strings.Contains(tagBlob, needle) {
			return true
		}
	}
	return false
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
	idxMALID := headerIndex(headers, "series_animedb_id", "series_anime_db_id", "anime_id", "mal_id")
	idxComments := headerIndex(headers, "my_comments", "comments", "notes")
	idxTags := headerIndex(headers, "my_tags", "tags")

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
			Source:    "mal",
			IsAdult:   malAnimeIsAdult(safeCell(row, idxType), safeCell(row, idxTags)),
		}
		if tot, err := strconv.Atoi(safeCell(row, idxTotal)); err == nil && tot > 0 {
			w.TotalEpisodes = &tot
		}
		if malID := strings.TrimSpace(safeCell(row, idxMALID)); malID != "" {
			if id, err := strconv.Atoi(malID); err == nil && id > 0 {
				w.ExternalID = strconv.Itoa(id)
				w.Link = "https://myanimelist.net/anime/" + w.ExternalID
			}
		}
		notes := strings.TrimSpace(safeCell(row, idxComments))
		if tags := strings.TrimSpace(safeCell(row, idxTags)); tags != "" {
			if notes != "" {
				notes += "\n"
			}
			notes += tags
		}
		w.Notes = notes
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
			IsAdult:    malAnimeIsAdult(e.SeriesType, e.MyTags),
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
