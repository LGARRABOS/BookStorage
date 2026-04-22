package server

import (
	"net/http"
	"strings"
)

const (
	maxChapterValue = 9999
	maxRatingValue  = 5
)

func sanitizeTitle(s string) string {
	return strings.TrimSpace(s)
}

func clampChapter(v int) int {
	if v < 0 {
		return 0
	}
	if v > maxChapterValue {
		return maxChapterValue
	}
	return v
}

func clampRating(v int) int {
	if v < 0 || v > maxRatingValue {
		return 0
	}
	return v
}

func normalizeStatusForWrite(raw string) string {
	s := normalizeStatus(raw)
	if !isValidStatus(s) {
		return "En cours"
	}
	return s
}

func normalizeReadingTypeForWrite(raw string) string {
	s := normalizeReadingType(raw)
	if !isValidReadingType(s) {
		return readingTypes[0]
	}
	return s
}

// notifyNewChaptersDB returns DB notify_new_chapters: 1 = suivi (hors filtre « Non suivis »), 0 = non-suivi.
// Hors statut « En cours », la valeur est toujours 1 (le marquage non-suivi ne s’applique qu’aux œuvres en cours).
func notifyNewChaptersDB(status string, suivi bool) int {
	if status != "En cours" {
		return 1
	}
	if suivi {
		return 1
	}
	return 0
}

func notifyNewChaptersFromForm(status string, r *http.Request) int {
	if r == nil {
		return 1
	}
	nonSuivi := r.FormValue("non_suivi") == "1" || strings.EqualFold(r.FormValue("non_suivi"), "on")
	return notifyNewChaptersDB(status, !nonSuivi)
}
