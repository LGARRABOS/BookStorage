package server

import "strings"

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
