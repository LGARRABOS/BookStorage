package server

import (
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
	ISBN       string `json:"isbn,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
}

func parseBdCSVRecords(records [][]string) ([]exportBdWork, bool) {
	if len(records) < 1 {
		return nil, false
	}
	headers := make([]string, len(records[0]))
	for i, h := range records[0] {
		headers[i] = normalizeHeader(h)
	}
	if isBdgestCSVHeader(headers) {
		out := parseBdgestCSVRecords(records, headers)
		return out, len(out) > 0
	}
	if isBdgestPositionalCSV(records) {
		out := parseBdgestPositionalCSVRecords(records)
		return out, len(out) > 0
	}
	if len(records) < 2 {
		return nil, false
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
	idxISBN := headerIndex(headers, "isbn", "ean")

	var out []exportBdWork
	for _, row := range records[1:] {
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
			ISBN:       safeCell(row, idxISBN),
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

// BDGest Online backup CSV (Outils → Sauvegarde), separator ';'.
// Documented columns include IdAlbum, ISBN, Serie, Num, NumA, Titre, … Note, Wishlist/AAcheter, Commentaire.
func isBdgestCSVHeader(headers []string) bool {
	hasTitre := headerIndex(headers, "titre") >= 0
	hasSerie := headerIndex(headers, "serie") >= 0
	hasID := headerIndex(headers, "idalbum", "id_album") >= 0
	hasISBN := headerIndex(headers, "isbn") >= 0
	return hasTitre && hasSerie && (hasID || hasISBN)
}

func isBdgestPositionalCSV(records [][]string) bool {
	if len(records) == 0 || len(records[0]) < 6 {
		return false
	}
	first := strings.TrimPrefix(strings.TrimSpace(records[0][0]), "\ufeff")
	if strings.EqualFold(first, "IdAlbum") || strings.EqualFold(first, "ISBN") {
		return false
	}
	if _, err := strconv.Atoi(first); err != nil {
		return false
	}
	// Positional layout needs Serie (2) and Titre (5) slots.
	serie := strings.TrimSpace(safeCell(records[0], 2))
	titre := strings.TrimSpace(safeCell(records[0], 5))
	return serie != "" || titre != ""
}

func bdgestTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "oui", "yes", "y", "x":
		return true
	default:
		return false
	}
}

func buildBdgestTitle(serie, titre string) string {
	serie = strings.TrimSpace(serie)
	titre = strings.TrimSpace(titre)
	switch {
	case serie != "" && titre != "" && !strings.EqualFold(serie, titre):
		return serie + " — " + titre
	case titre != "":
		return titre
	default:
		return serie
	}
}

func parseBdgestTome(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return clampChapter(n)
	}
	var digits strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
			continue
		}
		if digits.Len() > 0 {
			break
		}
	}
	if digits.Len() == 0 {
		return 0
	}
	n, _ := strconv.Atoi(digits.String())
	return clampChapter(n)
}

func mapBdgestFormat(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(s, "intégrale"), strings.Contains(s, "integrale"):
		return "Intégrale"
	case strings.Contains(s, "one"), strings.Contains(s, "oneshot"), strings.Contains(s, "one-shot"):
		return "One-shot"
	case strings.Contains(s, "série"), strings.Contains(s, "serie"):
		return "Série"
	default:
		return "Album"
	}
}

func parseBdgestCSVRecords(records [][]string, headers []string) []exportBdWork {
	idxID := headerIndex(headers, "idalbum", "id_album")
	idxISBN := headerIndex(headers, "isbn", "ean")
	idxSerie := headerIndex(headers, "serie")
	idxNum := headerIndex(headers, "num", "numero", "numéro")
	idxTitre := headerIndex(headers, "titre")
	idxNote := headerIndex(headers, "note")
	idxWishlist := headerIndex(headers, "wishlist")
	idxBuy := headerIndex(headers, "aaacheter", "a_acheter", "aacheter")
	idxComment := headerIndex(headers, "commentaire", "commentaires", "comment")
	idxFormat := headerIndex(headers, "format")

	var out []exportBdWork
	for i := 1; i < len(records); i++ {
		row := records[i]
		if w, ok := bdgestRowToWork(
			safeCell(row, idxID),
			safeCell(row, idxISBN),
			safeCell(row, idxSerie),
			safeCell(row, idxNum),
			safeCell(row, idxTitre),
			safeCell(row, idxNote),
			safeCell(row, idxWishlist),
			safeCell(row, idxBuy),
			safeCell(row, idxComment),
			safeCell(row, idxFormat),
		); ok {
			out = append(out, w)
		}
	}
	return out
}

// parseBdgestPositionalCSVRecords maps the documented backup column order when no header row is present.
func parseBdgestPositionalCSVRecords(records [][]string) []exportBdWork {
	var out []exportBdWork
	for i := 0; i < len(records); i++ {
		row := records[i]
		if w, ok := bdgestRowToWork(
			safeCell(row, 0),  // IdAlbum
			safeCell(row, 1),  // ISBN
			safeCell(row, 2),  // Serie
			safeCell(row, 3),  // Num
			safeCell(row, 5),  // Titre
			safeCell(row, 15), // Note
			safeCell(row, 18), // Wishlist
			"",                // AAcheter (not always present at fixed index)
			safeCell(row, 26), // Commentaire
			safeCell(row, 24), // Format
		); ok {
			out = append(out, w)
		}
	}
	return out
}

func bdgestRowToWork(id, isbn, serie, num, titre, note, wishlist, buy, comment, format string) (exportBdWork, bool) {
	title := buildBdgestTitle(serie, titre)
	if title == "" {
		return exportBdWork{}, false
	}
	status := "Terminé"
	if bdgestTruthy(wishlist) || bdgestTruthy(buy) {
		status = "À lire"
	}
	rating, _ := strconv.Atoi(strings.TrimSpace(note))
	w := exportBdWork{
		Title:      title,
		Tome:       parseBdgestTome(num),
		Status:     status,
		BdType:     mapBdgestFormat(format),
		Rating:     malScoreToStars(rating),
		Notes:      strings.TrimSpace(comment),
		Source:     "bdgest",
		ExternalID: strings.TrimSpace(id),
		ISBN:       strings.TrimSpace(isbn),
	}
	return w, true
}
