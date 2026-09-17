package recommend

import (
	"encoding/json"
	"sort"
	"strings"

	"bookstorage/internal/catalog"
)

const (
	maxPerRelatedWork   = 2
	maxPerPrimaryGenre  = 5
	dislikeScoreCoeff   = 1.15
	topGenreBonusCoeff  = 0.2
	typeMatchBoost      = 0.45
	typeMismatchPenalty = 0.2
)

func parseJSONStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{})
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func isDisliked(w userWork) bool {
	st := strings.TrimSpace(w.Status)
	if st == "Abandonné" || st == "Dropped" {
		return true
	}
	return w.Rating == 1 || w.Rating == 2
}

func splitTaste(list []weightedWork) (liked, disliked []weightedWork) {
	for _, w := range list {
		if isDisliked(w.userWork) {
			disliked = append(disliked, w)
			continue
		}
		liked = append(liked, w)
	}
	sort.Slice(disliked, func(i, j int) bool {
		if disliked[i].weight != disliked[j].weight {
			return disliked[i].weight < disliked[j].weight
		}
		return disliked[i].id < disliked[j].id
	})
	return liked, disliked
}

func mediaDetailFromLocal(w weightedWork) *catalog.MediaDetail {
	if len(w.Genres) == 0 && len(w.Tags) == 0 {
		return nil
	}
	d := &catalog.MediaDetail{
		ID:     w.id,
		Genres: append([]string(nil), w.Genres...),
	}
	for _, name := range w.Tags {
		d.Tags = append(d.Tags, catalog.MediaTag{Name: name, Rank: 50})
	}
	return d
}

type browsePlan struct {
	genreIn []string
	tagIn   []string
	sort    string
}

// defaultBrowsePlans avoids AniList tag_in AND of several tags (almost empty pages).
func defaultBrowsePlans(profile TasteProfile) []browsePlan {
	genres := topGenreNames(profile, browseGenreN)
	tags := topTagNames(profile, 1)
	var plans []browsePlan
	if len(genres) > 0 {
		plans = append(plans, browsePlan{genreIn: genres, sort: "SCORE_DESC"})
	}
	if len(tags) > 0 {
		plans = append(plans, browsePlan{tagIn: tags, sort: "SCORE_DESC"})
	}
	if len(genres) > 0 {
		plans = append(plans, browsePlan{genreIn: genres, sort: "POPULARITY_DESC"})
	}
	return plans
}

func dominantReadingTypes(works []weightedWork) []string {
	weights := map[string]float64{}
	var total float64
	for _, w := range works {
		rt := strings.TrimSpace(w.ReadingType)
		if rt == "" {
			continue
		}
		weights[rt] += w.weight
		total += w.weight
	}
	if total <= 0 {
		return nil
	}
	type kv struct {
		name   string
		weight float64
	}
	var list []kv
	for name, w := range weights {
		if w/total < 0.2 {
			continue
		}
		list = append(list, kv{name: name, weight: w})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].weight != list[j].weight {
			return list[i].weight > list[j].weight
		}
		return list[i].name < list[j].name
	})
	if len(list) == 0 {
		return nil
	}
	if list[0].weight/total >= singleTypeLockShare {
		return []string{list[0].name}
	}
	if len(list) > 2 {
		list = list[:2]
	}
	out := make([]string, len(list))
	for i, x := range list {
		out[i] = x.name
	}
	return out
}

func readingTypeBoost(candidateType string, preferred []string) float64 {
	if len(preferred) == 0 || strings.TrimSpace(candidateType) == "" {
		return 0
	}
	matched := false
	for _, p := range preferred {
		if strings.EqualFold(p, candidateType) {
			matched = true
			break
		}
	}
	if !matched {
		if len(preferred) == 1 {
			return -typeMismatchPenalty
		}
		return 0
	}
	boost := typeMatchBoost
	if strings.EqualFold(candidateType, webtoonTypeName) {
		boost += webtoonMatchExtra
	}
	return boost
}

func topGenreBonus(m profileMaps, genres []string, topName string) float64 {
	topName = strings.TrimSpace(topName)
	if topName == "" {
		return 0
	}
	for _, g := range genres {
		if strings.TrimSpace(g) != topName {
			continue
		}
		if w, ok := m.genres[topName]; ok {
			return w * topGenreBonusCoeff
		}
	}
	return 0
}

func finalizeScore(base, dislikeOverlap, typeBoost float64) float64 {
	s := base - dislikeOverlap*dislikeScoreCoeff + typeBoost
	if s < 0 {
		return 0
	}
	return s
}

func primaryMatchedGenre(s Suggestion) string {
	if len(s.MatchedGenres) > 0 {
		return s.MatchedGenres[0]
	}
	return ""
}

func diversify(cands []rankedCandidate, cap int) []Suggestion {
	if cap <= 0 {
		return nil
	}
	sorted := append([]rankedCandidate(nil), cands...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].score != sorted[j].score {
			return sorted[i].score > sorted[j].score
		}
		return sorted[i].suggestion.AnilistID < sorted[j].suggestion.AnilistID
	})
	relatedN := map[int]int{}
	genreN := map[string]int{}
	used := make([]bool, len(sorted))
	var picked []Suggestion

	tryPick := func(strict bool) bool {
		for i, c := range sorted {
			if used[i] {
				continue
			}
			rid := c.suggestion.RelatedAnilistID
			if rid > 0 && relatedN[rid] >= maxPerRelatedWork {
				continue
			}
			g := primaryMatchedGenre(c.suggestion)
			if g != "" && genreN[g] >= maxPerPrimaryGenre && strict {
				continue
			}
			used[i] = true
			picked = append(picked, c.suggestion)
			if rid > 0 {
				relatedN[rid]++
			}
			if g != "" {
				genreN[g]++
			}
			return true
		}
		return false
	}

	for len(picked) < cap {
		if tryPick(true) {
			continue
		}
		if tryPick(false) {
			continue
		}
		break
	}
	return picked
}

func mediaFetchTargetsN(liked []weightedWork, haveLocal map[int]*catalog.MediaDetail, edgeN int) []weightedWork {
	seen := map[int]struct{}{}
	var out []weightedWork
	add := func(w weightedWork) {
		if _, ok := seen[w.id]; ok {
			return
		}
		seen[w.id] = struct{}{}
		out = append(out, w)
	}
	n := len(liked)
	if edgeN < 0 {
		edgeN = 0
	}
	if n > edgeN {
		n = edgeN
	}
	for _, w := range liked[:n] {
		add(w)
	}
	for _, w := range liked {
		if len(out) >= maxMediaFetches {
			break
		}
		if _, ok := haveLocal[w.id]; ok {
			continue
		}
		add(w)
	}
	return out
}

func mediaFetchTargets(liked []weightedWork, haveLocal map[int]*catalog.MediaDetail) []weightedWork {
	return mediaFetchTargetsN(liked, haveLocal, maxTopForRecEdges)
}
