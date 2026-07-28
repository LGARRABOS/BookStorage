package server

import (
	"encoding/json"
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
	Type       string         `json:"type"`
	Title      aniImportTitle `json:"title"`
	Format     string         `json:"format"`
	IsAdult    bool           `json:"isAdult"`
	CoverImage aniImportCover `json:"coverImage"`
	Episodes   *int           `json:"episodes"`
}
type aniImportEntry struct {
	Status   string         `json:"status"`
	Progress int            `json:"progress"`
	Score    float64        `json:"score"`
	Notes    string         `json:"notes"`
	Media    aniImportMedia `json:"media"`
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

// sanitizeImportImagePath keeps relative upload paths; external URLs must be http(s) only.
func sanitizeImportImagePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, "data:") || strings.HasPrefix(path, "//") {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if strings.Contains(path, ":") && !strings.HasPrefix(path, "/") {
		return ""
	}
	return path
}

// csvSafeCell prefixes spreadsheet formula triggers (= + - @ tab CR) to prevent CSV injection on export.
func csvSafeCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	default:
		return s
	}
}

func appendImportError(report *ImportReport, line int, code string) {
	if len(report.Errors) >= 30 {
		return
	}
	report.Errors = append(report.Errors, ImportLineError{Line: line, Msg: code})
}

func mustJSON(rep ImportReport) []byte {
	b, err := json.Marshal(rep)
	if err != nil {
		return nil
	}
	return b
}

func normalizeHeader(s string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "\ufeff")
	s = strings.ToLower(s)
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

func parseDuplicateMode(s string) DuplicateMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(DuplicateUpdate), "merge", "overwrite":
		return DuplicateUpdate
	default:
		return DuplicateSkip
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
