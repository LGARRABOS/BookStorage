package catalog

import (
	"strings"

	"bookstorage/internal/database"
)

// UserBlocklist holds per-user genre and tag exclusions for catalog browse/search/recommend.
type UserBlocklist struct {
	Genres []string
	Tags   []string
}

// LoadUserBlocklist returns blocked genre and tag labels for a user.
func LoadUserBlocklist(db *database.Conn, userID int64) (UserBlocklist, error) {
	var out UserBlocklist
	if db == nil || userID <= 0 {
		return out, nil
	}
	rows, err := db.Query(
		`SELECT label_type, label_name FROM user_catalog_blocklist WHERE user_id = ? ORDER BY label_type, label_name`,
		userID,
	)
	if err != nil {
		return out, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var labelType, labelName string
		if err := rows.Scan(&labelType, &labelName); err != nil {
			return out, err
		}
		labelName = strings.TrimSpace(labelName)
		if labelName == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(labelType)) {
		case "genre":
			out.Genres = append(out.Genres, labelName)
		case "tag":
			out.Tags = append(out.Tags, labelName)
		}
	}
	return out, rows.Err()
}

// MergeBlocklistFilter combines user blocklist exclusions with an existing orientation filter.
func MergeBlocklistFilter(blocklist UserBlocklist, orient AdultOrientationFilter) AdultOrientationFilter {
	if len(blocklist.Genres) == 0 && len(blocklist.Tags) == 0 {
		return orient
	}
	tagNotIn := append(append([]string(nil), orient.TagNotIn...), blocklist.Tags...)
	prevMatch := orient.MatchMedia
	orient.TagNotIn = uniqueStrings(tagNotIn)
	orient.MatchMedia = func(genres, tags []string) bool {
		if mediaBlockedByBlocklist(blocklist, genres, tags) {
			return false
		}
		if prevMatch == nil {
			return true
		}
		return prevMatch(genres, tags)
	}
	return orient
}

func mediaBlockedByBlocklist(bl UserBlocklist, genres, tags []string) bool {
	labels := mediaLabels(genres, tags)
	if len(bl.Genres) > 0 && labelsMatchAny(labels, bl.Genres) {
		return true
	}
	if len(bl.Tags) > 0 && labelsMatchAny(labels, bl.Tags) {
		return true
	}
	return false
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := normalizeMediaLabel(s)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}
