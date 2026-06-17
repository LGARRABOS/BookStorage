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
	maxMediaFetches   = 8
	maxTopForRecEdges = 3
	browsePerPage     = 25
	browsePoolSize    = 24
	graphPoolPerWork  = 12
	finalCap          = 18
	profileTopN       = 5
	browseFilterN     = 3
)

// Weighted work row from SQL.
type userWork struct {
	AnilistID string
	Rating    int
	Status    string
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
		SELECT c.external_id, COALESCE(w.rating, 0), COALESCE(w.status, '')
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
		if err := rows.Scan(&w.AnilistID, &w.Rating, &w.Status); err != nil {
			return nil, err
		}
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
	Results []Suggestion   `json:"results"`
	Profile ProfileSummary `json:"profile"`
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

// ForUser returns ranked browse + graph recommendations, excluding owned and dismissed ids.
func ForUser(db *database.Conn, userID int64, cfg ForUserConfig) (*ForUserResult, error) {
	o := cfg.Options
	if o == (Options{}) {
		o = DefaultOptions()
	}
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

	nFetch := len(list)
	if nFetch > maxMediaFetches {
		nFetch = maxMediaFetches
	}
	detailsByID := make(map[int]*catalog.MediaDetail)
	var weights []float64
	var order []int
	for i := 0; i < nFetch; i++ {
		id := list[i].id
		d, err := catalog.GetMediaByID(id)
		if err != nil {
			return nil, err
		}
		detailsByID[id] = d
		weights = append(weights, list[i].weight)
		order = append(order, id)
	}
	var detailPtrs []*catalog.MediaDetail
	for _, id := range order {
		detailPtrs = append(detailPtrs, detailsByID[id])
	}
	profile := aggregateProfile(detailPtrs, weights)
	if len(profile.Genres) == 0 && len(profile.Tags) == 0 {
		return nil, nil
	}

	profTop := profileSummary(profile, profileTopN, profileTopN)
	pmaps := buildProfileMaps(profile)
	genreIn := topGenreNames(profile, browseFilterN)
	tagIn := topTagNames(profile, browseFilterN)

	seen := make(map[int]struct{})
	for id := range known {
		seen[id] = struct{}{}
	}
	for id := range cfg.DismissedIDs {
		seen[id] = struct{}{}
	}

	var pool []rankedCandidate

	appendBrowse := func(sort string) error {
		browse, _, err := catalog.BrowseMedia(catalog.BrowseMediaParams{
			GenreIn:    genreIn,
			TagIn:      tagIn,
			TagNotIn:   mediaFilter.TagNotIn,
			MediaMatch: mediaFilter.MatchMedia,
			Page:       1,
			PerPage:    browsePerPage,
			Sort:       sort,
			NotInIDs:   seen,
			MaxResults: browsePoolSize,
		})
		if err != nil {
			return err
		}
		for _, r := range browse {
			if _, dup := seen[r.ID]; dup {
				continue
			}
			seen[r.ID] = struct{}{}
			mg := intersectOrdered(r.Genres, profTop.TopGenres)
			mt := intersectOrdered(r.Tags, profTop.TopTags)
			overlap := profileOverlapScore(pmaps, r.Genres, r.Tags)
			pool = append(pool, rankedCandidate{
				suggestion: Suggestion{
					Source:        "browse",
					AnilistID:     r.ID,
					Title:         r.Title,
					ReadingType:   r.ReadingType,
					ImageURL:      r.ImageURL,
					IsAdult:       r.IsAdult,
					MatchedGenres: mg,
					MatchedTags:   mt,
				},
				score: candidateScore(overlap, "browse", 0, 0),
			})
		}
		return nil
	}

	if len(genreIn) > 0 || len(tagIn) > 0 {
		if err := appendBrowse("SCORE_DESC"); err != nil {
			_ = appendBrowse("POPULARITY_DESC")
		}
	}

	nRec := len(list)
	if nRec > maxTopForRecEdges {
		nRec = maxTopForRecEdges
	}
	for i := 0; i < nRec; i++ {
		src := list[i]
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
			seen[r.ID] = struct{}{}
			mg := intersectOrdered(r.Genres, profTop.TopGenres)
			mt := intersectOrdered(r.Tags, profTop.TopTags)
			overlap := profileOverlapScore(pmaps, r.Genres, r.Tags)
			pool = append(pool, rankedCandidate{
				suggestion: Suggestion{
					Source:           "recommendation",
					AnilistID:        r.ID,
					Title:            r.Title,
					ReadingType:      r.ReadingType,
					ImageURL:         r.ImageURL,
					IsAdult:          r.IsAdult,
					RelatedTitle:     relatedTitle,
					RelatedAnilistID: src.id,
					MatchedGenres:    mg,
					MatchedTags:      mt,
				},
				score: candidateScore(overlap, "recommendation", r.RecommendationRating, src.weight),
			})
			added++
			if added >= graphPoolPerWork {
				break
			}
		}
	}

	ranked := rankCandidates(pool)
	if len(ranked) > finalCap {
		ranked = ranked[:finalCap]
	}
	return &ForUserResult{Results: ranked, Profile: profTop}, nil
}
