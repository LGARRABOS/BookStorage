package server

import "strings"

// fts5MatchExpression builds a safe prefix MATCH string for SQLite FTS5 (token AND).
// Returns false if there is no usable token after trimming.
func fts5MatchExpression(search string) (string, bool) {
	parts := strings.Fields(strings.TrimSpace(search))
	var tokens []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = strings.ReplaceAll(p, `"`, `""`)
		tokens = append(tokens, `"`+p+`"*`)
	}
	if len(tokens) == 0 {
		return "", false
	}
	return strings.Join(tokens, " AND "), true
}
