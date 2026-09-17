package recommend

import (
	"strings"

	"bookstorage/internal/catalog"
)

const (
	webtoonTypeName     = "Webtoon"
	webtoonLockShare    = 0.4
	singleTypeLockShare = 0.55
	webtoonMatchExtra   = 0.25
	maxTopAdultRecEdges = 3
	laneCap             = 12
)

func boolPtr(v bool) *bool { return &v }

func workIsAdult(w weightedWork, details map[int]*catalog.MediaDetail) bool {
	if w.IsAdult {
		return true
	}
	if details == nil {
		return false
	}
	d := details[w.id]
	return d != nil && d.RawMedia.IsAdult
}

func partitionByAdult(list []weightedWork, details map[int]*catalog.MediaDetail) (sfw, adult []weightedWork) {
	for _, w := range list {
		if workIsAdult(w, details) {
			adult = append(adult, w)
			continue
		}
		sfw = append(sfw, w)
	}
	return sfw, adult
}

func profileFromWorks(liked []weightedWork, details map[int]*catalog.MediaDetail) TasteProfile {
	var ptrs []*catalog.MediaDetail
	var weights []float64
	for _, w := range liked {
		d := details[w.id]
		if d == nil {
			continue
		}
		ptrs = append(ptrs, d)
		weights = append(weights, w.weight)
	}
	return aggregateProfile(ptrs, weights)
}

func dislikeMapsFrom(disliked []weightedWork, details map[int]*catalog.MediaDetail, getMedia func(int) (*catalog.MediaDetail, error), fetchLimit int) profileMaps {
	local := make(map[int]*catalog.MediaDetail)
	for _, w := range disliked {
		if d := details[w.id]; d != nil {
			local[w.id] = d
			continue
		}
		if d := mediaDetailFromLocal(w); d != nil {
			local[w.id] = d
		}
	}
	fetched := 0
	for _, w := range disliked {
		if fetched >= fetchLimit {
			break
		}
		if _, ok := local[w.id]; ok {
			continue
		}
		if getMedia == nil {
			continue
		}
		d, err := getMedia(w.id)
		if err != nil || d == nil {
			continue
		}
		local[w.id] = d
		fetched++
	}
	var ptrs []*catalog.MediaDetail
	var weights []float64
	for _, w := range disliked {
		d := local[w.id]
		if d == nil {
			continue
		}
		ptrs = append(ptrs, d)
		weights = append(weights, 1)
	}
	return buildProfileMaps(aggregateProfile(ptrs, weights))
}

func browseReadingTypes(works []weightedWork) []string {
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
	if weights[webtoonTypeName]/total >= webtoonLockShare {
		return []string{webtoonTypeName}
	}
	return dominantReadingTypes(works)
}

func mergeFetchTargets(parts [][]weightedWork, cap int) []weightedWork {
	seen := map[int]struct{}{}
	var out []weightedWork
	for _, part := range parts {
		for _, w := range part {
			if cap > 0 && len(out) >= cap {
				return out
			}
			if _, ok := seen[w.id]; ok {
				continue
			}
			seen[w.id] = struct{}{}
			out = append(out, w)
		}
	}
	return out
}

type laneCtx struct {
	pmaps     profileMaps
	negMaps   profileMaps
	topGenre  string
	preferred []string
	profTop   ProfileSummary
}

func makeLaneCtx(profile TasteProfile, negMaps profileMaps, liked []weightedWork) laneCtx {
	sum := profileSummary(profile, profileTopN, profileTopN)
	top := ""
	if len(sum.TopGenres) > 0 {
		top = sum.TopGenres[0]
	}
	return laneCtx{
		pmaps:     buildProfileMaps(profile),
		negMaps:   negMaps,
		topGenre:  top,
		preferred: browseReadingTypes(liked),
		profTop:   sum,
	}
}

func suggestionFromBrowse(r catalog.AnilistResult, ctx laneCtx) rankedCandidate {
	return rankedCandidate{
		suggestion: Suggestion{
			Source:        "browse",
			AnilistID:     r.ID,
			Title:         r.Title,
			ReadingType:   r.ReadingType,
			ImageURL:      r.ImageURL,
			IsAdult:       r.IsAdult,
			MatchedGenres: intersectOrdered(r.Genres, ctx.profTop.TopGenres),
			MatchedTags:   intersectOrdered(r.Tags, ctx.profTop.TopTags),
		},
		score: scoreSuggestion(ctx.pmaps, ctx.negMaps, ctx.topGenre, ctx.preferred, r.Genres, r.Tags, r.ReadingType, "browse", 0, 0),
	}
}

func suggestionFromGraph(r catalog.AnilistResult, relatedTitle string, src weightedWork, ctx laneCtx) rankedCandidate {
	return rankedCandidate{
		suggestion: Suggestion{
			Source:           "recommendation",
			AnilistID:        r.ID,
			Title:            r.Title,
			ReadingType:      r.ReadingType,
			ImageURL:         r.ImageURL,
			IsAdult:          r.IsAdult,
			RelatedTitle:     relatedTitle,
			RelatedAnilistID: src.id,
			MatchedGenres:    intersectOrdered(r.Genres, ctx.profTop.TopGenres),
			MatchedTags:      intersectOrdered(r.Tags, ctx.profTop.TopTags),
		},
		score: scoreSuggestion(ctx.pmaps, ctx.negMaps, ctx.topGenre, ctx.preferred, r.Genres, r.Tags, r.ReadingType, "recommendation", r.RecommendationRating, src.weight),
	}
}
