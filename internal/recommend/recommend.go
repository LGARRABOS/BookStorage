// Package recommend builds personalized suggestions from AniList + local library links.
package recommend

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"bookstorage/internal/catalog"
)

const (
	maxMediaFetches   = 8
	maxTopForRecEdges = 3
	browsePerPage     = 14
	finalCap          = 18
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
func LoadUserAnilistWorks(db *sql.DB, userID int64) ([]userWork, error) {
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

// ForUser returns merged browse + graph recommendations, excluding already-owned ids.
func ForUser(db *sql.DB, userID int64, o Options) (*ForUserResult, error) {
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

	profTop := profileSummary(profile, 5, 5)

	var genreIn []string
	for i, g := range profile.Genres {
		if i >= 2 {
			break
		}
		genreIn = append(genreIn, g.Name)
	}
	var tagIn []string
	for i, t := range profile.Tags {
		if i >= 2 {
			break
		}
		tagIn = append(tagIn, t.Name)
	}

	seen := make(map[int]struct{})
	for id := range known {
		seen[id] = struct{}{}
	}
	var out []Suggestion

	if len(genreIn) > 0 {
		browse, err := catalog.BrowseMedia(catalog.BrowseMediaParams{
			GenreIn:    genreIn,
			TagIn:      tagIn,
			Page:       1,
			PerPage:    browsePerPage,
			Sort:       "POPULARITY_DESC",
			NotInIDs:   seen,
			MaxResults: browsePerPage / 2,
		})
		if err != nil {
			return nil, fmt.Errorf("browse: %w", err)
		}
		for _, r := range browse {
			if _, dup := seen[r.ID]; dup {
				continue
			}
			seen[r.ID] = struct{}{}
			mg := intersectOrdered(r.Genres, profTop.TopGenres)
			mt := intersectOrdered(r.Tags, profTop.TopTags)
			out = append(out, Suggestion{
				Source:        "browse",
				AnilistID:     r.ID,
				Title:         r.Title,
				ReadingType:   r.ReadingType,
				ImageURL:      r.ImageURL,
				IsAdult:       r.IsAdult,
				MatchedGenres: mg,
				MatchedTags:   mt,
			})
		}
	}

	nRec := len(list)
	if nRec > maxTopForRecEdges {
		nRec = maxTopForRecEdges
	}
	for i := 0; i < nRec; i++ {
		id := list[i].id
		d := detailsByID[id]
		if d == nil {
			continue
		}
		for _, r := range d.Recommendations {
			if _, dup := seen[r.ID]; dup {
				continue
			}
			seen[r.ID] = struct{}{}
			out = append(out, Suggestion{
				Source:      "recommendation",
				AnilistID:   r.ID,
				Title:       r.Title,
				ReadingType: r.ReadingType,
				ImageURL:    r.ImageURL,
				IsAdult:     r.IsAdult,
			})
			if len(out) >= finalCap {
				break
			}
		}
		if len(out) >= finalCap {
			break
		}
	}

	if len(out) > finalCap {
		out = out[:finalCap]
	}
	return &ForUserResult{Results: out, Profile: profTop}, nil
}
