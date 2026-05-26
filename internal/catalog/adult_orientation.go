package catalog

import "strings"

const (
	AdultOrientHetero  = "hetero"
	AdultOrientGay     = "gay"
	AdultOrientLesbian = "lesbian"
)

var gayOrientationTags = []string{"Boys Love", "Yaoi", "Shounen Ai"}
var lesbianOrientationTags = []string{"Girls Love", "Yuri", "Shoujo Ai"}

// LGBT tags excluded for hetero-only AniList queries.
var lgbtOrientationTags []string

func init() {
	seen := make(map[string]struct{})
	for _, t := range append(append([]string{}, gayOrientationTags...), lesbianOrientationTags...) {
		key := strings.ToLower(t)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		lgbtOrientationTags = append(lgbtOrientationTags, t)
	}
}

// AdultOrientationFilter maps user-facing +18 orientation picks to AniList tag filters.
type AdultOrientationFilter struct {
	TagIn     []string
	TagNotIn  []string
	MatchFunc func(tagNames []string) bool
}

// FilterValidAdultOrientations keeps known orientation ids (hetero, gay, lesbian).
func FilterValidAdultOrientations(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		AdultOrientHetero:  {},
		AdultOrientGay:     {},
		AdultOrientLesbian: {},
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(strings.ToLower(raw))
		if v == "" {
			continue
		}
		if _, ok := allowed[v]; !ok {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// ResolveAdultOrientationFilter builds AniList tag filters for selected orientations.
// Empty selection means no orientation filter (all +18).
func ResolveAdultOrientationFilter(selected []string) AdultOrientationFilter {
	if len(selected) == 0 {
		return AdultOrientationFilter{}
	}
	has := map[string]bool{}
	for _, s := range selected {
		has[s] = true
	}
	hasHetero := has[AdultOrientHetero]
	hasGay := has[AdultOrientGay]
	hasLesbian := has[AdultOrientLesbian]

	if hasHetero && !hasGay && !hasLesbian {
		return AdultOrientationFilter{TagNotIn: append([]string(nil), lgbtOrientationTags...)}
	}
	if hasHetero {
		return AdultOrientationFilter{
			MatchFunc: func(tags []string) bool {
				isGay := tagsMatchAny(tags, gayOrientationTags)
				isLesbian := tagsMatchAny(tags, lesbianOrientationTags)
				if hasGay && isGay {
					return true
				}
				if hasLesbian && isLesbian {
					return true
				}
				if hasHetero && !isGay && !isLesbian {
					return true
				}
				return false
			},
		}
	}
	if hasGay && hasLesbian {
		return AdultOrientationFilter{TagIn: append(append([]string{}, gayOrientationTags...), lesbianOrientationTags...)}
	}
	if hasGay {
		return AdultOrientationFilter{TagIn: append([]string(nil), gayOrientationTags...)}
	}
	if hasLesbian {
		return AdultOrientationFilter{TagIn: append([]string(nil), lesbianOrientationTags...)}
	}
	return AdultOrientationFilter{}
}

func tagsMatchAny(tags, needles []string) bool {
	set := make(map[string]struct{}, len(needles))
	for _, n := range needles {
		set[strings.ToLower(strings.TrimSpace(n))] = struct{}{}
	}
	for _, t := range tags {
		if _, ok := set[strings.ToLower(strings.TrimSpace(t))]; ok {
			return true
		}
	}
	return false
}
