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

// notifyNewChaptersDB returns 1 if chapter notifications are enabled for this work, 0 otherwise.
// For statuses other than "En cours", always 1 (non-suivi only applies to in-progress works).
func notifyNewChaptersDB(status string, wantNotify bool) int {
	if status != "En cours" {
		return 1
	}
	if wantNotify {
		return 1
	}
	return 0
}

func notifyNewChaptersFromForm(status string, r *http.Request) int {
	if r == nil {
		return 1
	}
	want := r.FormValue("notify_new_chapters") == "1" || strings.EqualFold(r.FormValue("notify_new_chapters"), "on")
	return notifyNewChaptersDB(status, want)
}
