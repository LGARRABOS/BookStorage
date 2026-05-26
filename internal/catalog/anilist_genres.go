package catalog

import "strings"

// anilistGenres is the set of genre names accepted by AniList GraphQL (English).
var anilistGenres = []string{
	"Action",
	"Adventure",
	"Comedy",
	"Drama",
	"Ecchi",
	"Fantasy",
	"Horror",
	"Mahou Shoujo",
	"Mecha",
	"Music",
	"Mystery",
	"Psychological",
	"Romance",
	"Sci-Fi",
	"Slice of Life",
	"Sports",
	"Supernatural",
	"Suspense",
}

var anilistGenreSet map[string]struct{}

func init() {
	anilistGenreSet = make(map[string]struct{}, len(anilistGenres))
	for _, g := range anilistGenres {
		anilistGenreSet[g] = struct{}{}
	}
}

// AnilistGenres returns the list of browseable AniList manga genres.
func AnilistGenres() []string {
	out := make([]string, len(anilistGenres))
	copy(out, anilistGenres)
	return out
}

// IsValidAnilistGenre reports whether name is a known AniList genre label.
func IsValidAnilistGenre(name string) bool {
	_, ok := anilistGenreSet[strings.TrimSpace(name)]
	return ok
}

// FilterValidAnilistGenres keeps at most max genres that appear in the whitelist.
func FilterValidAnilistGenres(in []string, max int) []string {
	if max <= 0 {
		max = 3
	}
	seen := make(map[string]struct{})
	var out []string
	for _, raw := range in {
		g := strings.TrimSpace(raw)
		if g == "" || !IsValidAnilistGenre(g) {
			continue
		}
		if _, dup := seen[g]; dup {
			continue
		}
		seen[g] = struct{}{}
		out = append(out, g)
		if len(out) >= max {
			break
		}
	}
	return out
}
