package catalog

import (
	"html"
	"regexp"
	"strings"
)

var reHTMLTag = regexp.MustCompile(`(?s)<[^>]+>`)
var reWhitespace = regexp.MustCompile(`\s+`)

// StripHTML removes tags and normalizes whitespace (AniList descriptions are HTML).
func StripHTML(s string) string {
	if s == "" {
		return ""
	}
	t := reHTMLTag.ReplaceAllString(s, " ")
	t = html.UnescapeString(t)
	return strings.TrimSpace(reWhitespace.ReplaceAllString(t, " "))
}
