// Package recommend builds personalized suggestions from AniList + local library links.
package recommend

import (
	"sort"
	"strconv"
	"strings"

	"bookstorage/internal/catalog"
	"bookstorage/internal/database"
)

const (
	maxMediaFetches    = 8
	maxTopForRecEdges  = 5
	maxDislikedFetches = 2
	browsePerPage      = 25
	browsePoolSize     = 24
	graphPoolPerWork   = 12
	profileTopN        = 5
	browseGenreN       = 2
)

// Weighted work row from SQL.
type userWork struct {
	AnilistID   string
	Rating      int
	Status      string
	ReadingType string
	Genres      []string
	Tags        []string
	IsAdult     bool
}

// Options tune scoring (exported for tests).
type Options struct {
	RatingWeightHigh float64
	RatingWeightMid  float64
	RatingWeightLow  float64
	RatingWeightNone float64
	StatusCompleted  float64
	StatusReading    float64
	StatusPlanned    float64
	StatusDropped    float64
	StatusOnHold     float64
}

// DefaultOptions matches French UI statuses in BookStorage.
func DefaultOptions() Options {
	return Options{
		RatingWeightHigh: 2.0,
		RatingWeightMid:  1.0,
		RatingWeightLow:  0.4,
		RatingWeightNone: 0.6,
		StatusCompleted:  1.0,
		StatusReading:    0.95,
		StatusPlanned:    0.75,
		StatusDropped:    0.25,
		StatusOnHold:     0.6,
	}
}

func statusMultiplier(st string, o Options) float64 {
	switch st {
	case "Terminé", "Completed":
		return o.StatusCompleted
	case "En cours", "Reading":
		return o.StatusReading
	case "À lire", "Plan to Read":
		return o.StatusPlanned
	case "Abandonné", "Dropped":
		return o.StatusDropped
	case "En pause", "On Hold":
		return o.StatusOnHold
	default:
		return 0.7
	}
}

func ratingMultiplier(r int, o Options) float64 {
	switch {
	case r >= 4:
		return o.RatingWeightHigh
	case r == 3:
		return o.RatingWeightMid
	case r >= 1:
		return o.RatingWeightLow
	default:
		return o.RatingWeightNone
	}
}

// LoadUserAnilistWorks returns catalog-linked AniList external ids with rating and status.
func LoadUserAnilistWorks(db *database.Conn, userID int64) ([]userWork, error) {
	rows, err := db.Query(`
		SELECT c.external_id,
		       COALESCE(w.rating, 0),
		       COALESCE(w.status, ''),
		       COALESCE(w.reading_type, ''),
		       COALESCE(c.genres, ''),
		       COALESCE(c.tags, ''),
		       COALESCE(w.is_adult, 0)
		FROM works w
		INNER JOIN catalog c ON c.id = w.catalog_id
		WHERE w.user_id = ? AND c.source = 'anilist' AND c.external_id != '' AND TRIM(c.external_id) != ''
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []userWork
	for rows.Next() {
		var w userWork
		var genresJSON, tagsJSON string
		var adult int
		if err := rows.Scan(&w.AnilistID, &w.Rating, &w.Status, &w.ReadingType, &genresJSON, &tagsJSON, &adult); err != nil {
			return nil, err
		}
		w.IsAdult = adult != 0
		w.Genres = parseJSONStringList(genresJSON)
		w.Tags = parseJSONStringList(tagsJSON)
		out = append(out, w)
	}
	return out, rows.Err()
}

// CollectKnownAnilistIDs parses external ids for exclusion.
func CollectKnownAnilistIDs(works []userWork) map[int]struct{} {
	m := make(map[int]struct{})
	for _, w := range works {
		id, err := strconv.Atoi(strings.TrimSpace(w.AnilistID))
		if err == nil && id > 0 {
			m[id] = struct{}{}
		}
	}
	return m
}

type weightedWork struct {
	userWork
	weight float64
	id     int
}

func buildWeightedList(works []userWork, o Options) []weightedWork {
	var list []weightedWork
	for _, w := range works {
		id, err := strconv.Atoi(strings.TrimSpace(w.AnilistID))
		if err != nil || id <= 0 {
			continue
		}
		s := ratingMultiplier(w.Rating, o) * statusMultiplier(w.Status, o)
		list = append(list, weightedWork{userWork: w, weight: s, id: id})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].weight != list[j].weight {
			return list[i].weight > list[j].weight
		}
		return list[i].id < list[j].id
	})
	return list
}

type scoredGenre struct {
	Name  string
	Score float64
}

type scoredTag struct {
	Name  string
	Score float64
}

// TasteProfile holds aggregated genre/tag scores.
type TasteProfile struct {
	Genres []scoredGenre
	Tags   []scoredTag
}

func aggregateProfile(details []*catalog.MediaDetail, weights []float64) TasteProfile {
	genreScores := map[string]float64{}
	tagScores := map[string]float64{}
	for i, d := range details {
		if d == nil || i >= len(weights) {
			continue
		}
		w := weights[i]
		for _, g := range d.Genres {
			g = strings.TrimSpace(g)
			if g == "" {
				continue
			}
			genreScores[g] += w
		}
		for _, t := range d.Tags {
			name := strings.TrimSpace(t.Name)
			if name == "" {
				continue
			}
			rank := float64(t.Rank)
			if rank <= 0 {
				rank = 50
			}
			tagScores[name] += w * (rank / 100.0)
		}
	}
	tp := TasteProfile{}
	for g, s := range genreScores {
		tp.Genres = append(tp.Genres, scoredGenre{Name: g, Score: s})
	}
	for t, s := range tagScores {
		tp.Tags = append(tp.Tags, scoredTag{Name: t, Score: s})
	}
	sort.Slice(tp.Genres, func(i, j int) bool {
		if tp.Genres[i].Score != tp.Genres[j].Score {
			return tp.Genres[i].Score > tp.Genres[j].Score
		}
		return tp.Genres[i].Name < tp.Genres[j].Name
	})
	sort.Slice(tp.Tags, func(i, j int) bool {
		if tp.Tags[i].Score != tp.Tags[j].Score {
			return tp.Tags[i].Score > tp.Tags[j].Score
		}
		return tp.Tags[i].Name < tp.Tags[j].Name
	})
	return tp
}

func profileSummary(p TasteProfile, maxG, maxT int) ProfileSummary {
	var g, t []string
	for i := 0; i < len(p.Genres) && i < maxG; i++ {
		g = append(g, p.Genres[i].Name)
	}
	for i := 0; i < len(p.Tags) && i < maxT; i++ {
		t = append(t, p.Tags[i].Name)
	}
	return ProfileSummary{TopGenres: g, TopTags: t}
}

// intersectOrdered keeps order from item; only includes values present in profileTop (trimmed).
func intersectOrdered(item []string, profileTop []string) []string {
	set := make(map[string]struct{})
	for _, p := range profileTop {
		p = strings.TrimSpace(p)
		if p != "" {
			set[p] = struct{}{}
		}
	}
	var out []string
	seen := make(map[string]struct{})
	for _, x := range item {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		if _, ok := set[x]; !ok {
			continue
		}
		if _, dup := seen[x]; dup {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}

// ForUserConfig tunes recommendation generation for one user.
type ForUserConfig struct {
	Options      Options
	DismissedIDs map[int]struct{}
	GetMedia     func(id int) (*catalog.MediaDetail, error)
	Browse       func(p catalog.BrowseMediaParams) ([]catalog.AnilistResult, int, error)
}

// DefaultForUserConfig returns standard scoring with no extra exclusions.
func DefaultForUserConfig() ForUserConfig {
	return ForUserConfig{Options: DefaultOptions()}
}

type profileMaps struct {
	genres map[string]float64
	tags   map[string]float64
}

func buildProfileMaps(p TasteProfile) profileMaps {
	gm := make(map[string]float64, len(p.Genres))
	for _, g := range p.Genres {
		gm[g.Name] = g.Score
	}
	tm := make(map[string]float64, len(p.Tags))
	for _, t := range p.Tags {
		tm[t.Name] = t.Score
	}
	return profileMaps{genres: gm, tags: tm}
}

// profileOverlapScore sums taste-profile weights for matching genres and tags on a candidate.
func profileOverlapScore(m profileMaps, genres, tags []string) float64 {
	var score float64
	seenG := make(map[string]struct{})
	for _, g := range genres {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if _, dup := seenG[g]; dup {
			continue
		}
		seenG[g] = struct{}{}
		if w, ok := m.genres[g]; ok {
			score += w
		}
	}
	seenT := make(map[string]struct{})
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, dup := seenT[t]; dup {
			continue
		}
		seenT[t] = struct{}{}
		if w, ok := m.tags[t]; ok {
			score += w * 0.85
		}
	}
	return score
}

type rankedCandidate struct {
	suggestion Suggestion
	score      float64
}

func rankCandidates(cands []rankedCandidate) []Suggestion {
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		return cands[i].suggestion.AnilistID < cands[j].suggestion.AnilistID
	})
	out := make([]Suggestion, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.suggestion)
	}
	return out
}

func candidateScore(overlap float64, source string, edgeRating int, sourceWorkWeight float64) float64 {
	score := overlap
	switch source {
	case "recommendation":
		edgeBoost := float64(edgeRating) / 100.0
		if edgeBoost <= 0 {
			edgeBoost = 0.55
		}
		score += edgeBoost * sourceWorkWeight * 1.4
		score += sourceWorkWeight * 0.25
	default:
		score += 0.1
	}
	return score
}

func topGenreNames(p TasteProfile, n int) []string {
	var out []string
	for i := 0; i < len(p.Genres) && i < n; i++ {
		out = append(out, p.Genres[i].Name)
	}
	return out
}

func topTagNames(p TasteProfile, n int) []string {
	var out []string
	for i := 0; i < len(p.Tags) && i < n; i++ {
		out = append(out, p.Tags[i].Name)
	}
	return out
}

// ProfileSummary is a short view of inferred taste for API clients.
type ProfileSummary struct {
	TopGenres []string `json:"top_genres"`
	TopTags   []string `json:"top_tags"`
}

// ForUserResult bundles suggestions and profile hints for UI copy.
type ForUserResult struct {
	Results        []Suggestion   `json:"results"`
	AdultResults   []Suggestion   `json:"adult_results"`
	Profile        ProfileSummary `json:"profile"`
	AdultProfile   ProfileSummary `json:"adult_profile"`
	AdultAvailable bool           `json:"adult_available"`
}

// Suggestion is one recommended title for API/JSON.
type Suggestion struct {
	Source           string   `json:"source"`
	AnilistID        int      `json:"anilist_id"`
	Title            string   `json:"title"`
	ReadingType      string   `json:"reading_type"`
	ImageURL         string   `json:"image_url,omitempty"`
	IsAdult          bool     `json:"is_adult"`
	RelatedTitle     string   `json:"related_title,omitempty"`
	RelatedAnilistID int      `json:"related_anilist_id,omitempty"`
	MatchedGenres    []string `json:"matched_genres,omitempty"`
	MatchedTags      []string `json:"matched_tags,omitempty"`
}

func resolveMediaGetter(cfg ForUserConfig) func(int) (*catalog.MediaDetail, error) {
	if cfg.GetMedia != nil {
		return cfg.GetMedia
	}
	return catalog.GetMediaByID
}

func resolveBrowse(cfg ForUserConfig) func(catalog.BrowseMediaParams) ([]catalog.AnilistResult, int, error) {
	if cfg.Browse != nil {
		return cfg.Browse
	}
	return catalog.BrowseMedia
}

func scoreSuggestion(pmaps, negMaps profileMaps, topGenre string, preferredTypes []string, genres, tags []string, readingType, source string, edgeRating int, sourceWeight float64) float64 {
	overlap := profileOverlapScore(pmaps, genres, tags) + topGenreBonus(pmaps, genres, topGenre)
	dislike := profileOverlapScore(negMaps, genres, tags)
	base := candidateScore(overlap, source, edgeRating, sourceWeight)
	return finalizeScore(base, dislike, readingTypeBoost(readingType, preferredTypes))
}

// ForUser returns ranked browse + graph recommendations, excluding owned and dismissed ids.
func ForUser(db *database.Conn, userID int64, cfg ForUserConfig) (*ForUserResult, error) {
	o := cfg.Options
	if o == (Options{}) {
		o = DefaultOptions()
	}
	getMedia := resolveMediaGetter(cfg)
	browseFn := resolveBrowse(cfg)
	blocklist, _ := catalog.LoadUserBlocklist(db, userID)
	mediaFilter := catalog.MergeBlocklistFilter(blocklist, catalog.AdultOrientationFilter{})

	works, err := LoadUserAnilistWorks(db, userID)
	if err != nil {
		return nil, err
	}
	known := CollectKnownAnilistIDs(works)
	if len(works) == 0 {
		return nil, nil
	}
	list := buildWeightedList(works, o)
	if len(list) == 0 {
		return nil, nil
	}
	liked, disliked := splitTaste(list)
	if len(liked) == 0 {
		return nil, nil
	}

	detailsByID := make(map[int]*catalog.MediaDetail)
	for _, w := range liked {
		if d := mediaDetailFromLocal(w); d != nil {
			detailsByID[w.id] = d
		}
	}
	sfwGuess, adultGuess := partitionByAdult(liked, detailsByID)
	fetchList := mergeFetchTargets([][]weightedWork{
		mediaFetchTargetsN(sfwGuess, detailsByID, maxTopForRecEdges),
		mediaFetchTargetsN(adultGuess, detailsByID, maxTopAdultRecEdges),
	}, maxMediaFetches)

	var lastFetchErr error
	for _, w := range fetchList {
		d, err := getMedia(w.id)
		if err != nil {
			lastFetchErr = err
			continue
		}
		if d != nil {
			detailsByID[w.id] = d
		}
	}

	sfwLiked, adultLiked := partitionByAdult(liked, detailsByID)
	sfwDisliked, adultDisliked := partitionByAdult(disliked, detailsByID)

	sfwProfile := profileFromWorks(sfwLiked, detailsByID)
	adultProfile := profileFromWorks(adultLiked, detailsByID)
	if len(sfwProfile.Genres) == 0 && len(sfwProfile.Tags) == 0 && len(adultProfile.Genres) == 0 && len(adultProfile.Tags) == 0 {
		if lastFetchErr != nil {
			return nil, lastFetchErr
		}
		return nil, nil
	}

	sfwNeg := dislikeMapsFrom(sfwDisliked, detailsByID, getMedia, maxDislikedFetches)
	adultNeg := dislikeMapsFrom(adultDisliked, detailsByID, getMedia, maxDislikedFetches)
	sfwCtx := makeLaneCtx(sfwProfile, sfwNeg, sfwLiked)
	adultCtx := makeLaneCtx(adultProfile, adultNeg, adultLiked)
	hasAdultLane := len(adultLiked) > 0 && (len(adultProfile.Genres) > 0 || len(adultProfile.Tags) > 0)

	seen := make(map[int]struct{})
	for id := range known {
		seen[id] = struct{}{}
	}
	for id := range cfg.DismissedIDs {
		seen[id] = struct{}{}
	}

	var sfwPool, adultPool []rankedCandidate
	appendBrowse := func(plan browsePlan, adult bool, ctx laneCtx, pool *[]rankedCandidate) error {
		browse, _, err := browseFn(catalog.BrowseMediaParams{
			GenreIn:        plan.genreIn,
			TagIn:          plan.tagIn,
			TagNotIn:       mediaFilter.TagNotIn,
			MediaMatch:     mediaFilter.MatchMedia,
			Page:           1,
			PerPage:        browsePerPage,
			Sort:           plan.sort,
			NotInIDs:       seen,
			MaxResults:     browsePoolSize,
			ReadingTypesIn: ctx.preferred,
			IsAdult:        boolPtr(adult),
		})
		if err != nil {
			return err
		}
		for _, r := range browse {
			if r.IsAdult != adult {
				continue
			}
			if _, dup := seen[r.ID]; dup {
				continue
			}
			seen[r.ID] = struct{}{}
			*pool = append(*pool, suggestionFromBrowse(r, ctx))
		}
		return nil
	}

	if len(sfwProfile.Genres) > 0 || len(sfwProfile.Tags) > 0 {
		for _, plan := range defaultBrowsePlans(sfwProfile) {
			if len(sfwPool) >= browsePoolSize {
				break
			}
			_ = appendBrowse(plan, false, sfwCtx, &sfwPool)
		}
	}
	if hasAdultLane {
		for _, plan := range defaultBrowsePlans(adultProfile) {
			if len(adultPool) >= browsePoolSize {
				break
			}
			_ = appendBrowse(plan, true, adultCtx, &adultPool)
		}
	}

	appendGraph := func(sources []weightedWork, edgeN int) {
		n := len(sources)
		if n > edgeN {
			n = edgeN
		}
		for _, src := range sources[:n] {
			d := detailsByID[src.id]
			if d == nil {
				continue
			}
			relatedTitle := d.Title
			added := 0
			for _, r := range d.Recommendations {
				if _, dup := seen[r.ID]; dup {
					continue
				}
				if mediaFilter.MatchMedia != nil && !mediaFilter.MatchMedia(r.Genres, r.Tags) {
					continue
				}
				if r.IsAdult {
					if !hasAdultLane {
						continue
					}
					seen[r.ID] = struct{}{}
					adultPool = append(adultPool, suggestionFromGraph(r, relatedTitle, src, adultCtx))
				} else {
					seen[r.ID] = struct{}{}
					sfwPool = append(sfwPool, suggestionFromGraph(r, relatedTitle, src, sfwCtx))
				}
				added++
				if added >= graphPoolPerWork {
					break
				}
			}
		}
	}
	appendGraph(sfwLiked, maxTopForRecEdges)
	if hasAdultLane {
		appendGraph(adultLiked, maxTopAdultRecEdges)
	}

	out := &ForUserResult{
		Results:        diversify(sfwPool, laneCap),
		AdultResults:   diversify(adultPool, laneCap),
		Profile:        sfwCtx.profTop,
		AdultProfile:   adultCtx.profTop,
		AdultAvailable: len(adultLiked) > 0,
	}
	if !hasAdultLane {
		out.AdultResults = []Suggestion{}
		if !out.AdultAvailable {
			out.AdultProfile = ProfileSummary{}
		}
	}
	return out, nil
}
