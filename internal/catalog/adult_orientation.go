package catalog

import "strings"

// User-facing filter ids (API: adult_orient=...).
const (
	AdultOrientHeterosexual = "heterosexual"
	AdultOrientBoysLove     = "boys_love"
	AdultOrientYuri         = "yuri"
)

// Exact AniList MediaTag names (MediaTagCollection).
const (
	anilistTagBoysLove     = "Boys' Love"
	anilistTagYuri         = "Yuri"
	anilistTagHeterosexual = "Heterosexual"
	anilistTagLGBTQThemes  = "LGBTQ+ Themes"
)

var boysLoveLabels = []string{anilistTagBoysLove, anilistTagLGBTQThemes}
var yuriLabels = []string{anilistTagYuri}
var lgbtExcludeLabels = []string{anilistTagBoysLove, anilistTagYuri, anilistTagLGBTQThemes}

// AdultOrientationFilter maps +18 orientation picks to AniList tag filters.
type AdultOrientationFilter struct {
	TagIn      []string
	TagNotIn   []string
	MatchMedia func(genres, tags []string) bool
}

// FilterValidAdultOrientations keeps known orientation ids.
func FilterValidAdultOrientations(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	allowed := map[string]string{
		AdultOrientHeterosexual: AdultOrientHeterosexual,
		AdultOrientBoysLove:     AdultOrientBoysLove,
		AdultOrientYuri:         AdultOrientYuri,
		"hetero":                AdultOrientHeterosexual,
		"gay":                   AdultOrientBoysLove,
		"lesbian":               AdultOrientYuri,
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(strings.ToLower(raw))
		if v == "" {
			continue
		}
		canon, ok := allowed[v]
		if !ok {
			continue
		}
		if _, dup := seen[canon]; dup {
			continue
		}
		seen[canon] = struct{}{}
		out = append(out, canon)
	}
	return out
}

// ResolveAdultOrientationFilter builds AniList tag filters for selected orientations.
func ResolveAdultOrientationFilter(selected []string) AdultOrientationFilter {
	if len(selected) == 0 {
		return AdultOrientationFilter{}
	}
	has := map[string]bool{}
	for _, s := range selected {
		has[s] = true
	}
	hasHetero := has[AdultOrientHeterosexual]
	hasBL := has[AdultOrientBoysLove]
	hasYuri := has[AdultOrientYuri]

	matchCombined := func(genres, tags []string) bool {
		labels := mediaLabels(genres, tags)
		isBL := labelsMatchAny(labels, boysLoveLabels)
		isYuri := labelsMatchAny(labels, yuriLabels)
		isHetero := labelsMatchAny(labels, []string{anilistTagHeterosexual}) || (!isBL && !isYuri)
		if hasBL && isBL {
			return true
		}
		if hasYuri && isYuri {
			return true
		}
		if hasHetero && isHetero && !isBL && !isYuri {
			return true
		}
		return false
	}

	if hasHetero && !hasBL && !hasYuri {
		return AdultOrientationFilter{
			TagNotIn: append([]string(nil), lgbtExcludeLabels...),
			MatchMedia: func(genres, tags []string) bool {
				labels := mediaLabels(genres, tags)
				return !labelsMatchAny(labels, lgbtExcludeLabels)
			},
		}
	}
	if hasHetero {
		return AdultOrientationFilter{MatchMedia: matchCombined}
	}
	if hasBL && hasYuri {
		return AdultOrientationFilter{
			TagIn: []string{anilistTagBoysLove, anilistTagYuri},
			MatchMedia: func(genres, tags []string) bool {
				labels := mediaLabels(genres, tags)
				return labelsMatchAny(labels, boysLoveLabels) || labelsMatchAny(labels, yuriLabels)
			},
		}
	}
	if hasBL {
		return AdultOrientationFilter{
			TagIn: []string{anilistTagBoysLove},
			MatchMedia: func(genres, tags []string) bool {
				return labelsMatchAny(mediaLabels(genres, tags), boysLoveLabels)
			},
		}
	}
	if hasYuri {
		return AdultOrientationFilter{
			TagIn: []string{anilistTagYuri},
			MatchMedia: func(genres, tags []string) bool {
				return labelsMatchAny(mediaLabels(genres, tags), yuriLabels)
			},
		}
	}
	return AdultOrientationFilter{}
}

func mediaLabels(genres, tags []string) []string {
	out := make([]string, 0, len(genres)+len(tags))
	out = append(out, genres...)
	out = append(out, tags...)
	return out
}

func normalizeMediaLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("'", "", "'", "", "’", "", " ", "").Replace(s)
	return s
}

func labelsMatchAny(labels, needles []string) bool {
	set := make(map[string]struct{}, len(needles))
	for _, n := range needles {
		set[normalizeMediaLabel(n)] = struct{}{}
	}
	for _, label := range labels {
		if _, ok := set[normalizeMediaLabel(label)]; ok {
			return true
		}
	}
	return false
}
