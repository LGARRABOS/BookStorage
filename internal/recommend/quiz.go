package recommend

import (
	"strings"

	"bookstorage/internal/catalog"
	"bookstorage/internal/database"
)

const (
	quizLaneSFW   = "sfw"
	quizLaneAdult = "adult"
	quizTagSmut   = "Smut"
	quizCap       = 12
)

// QuizRequest is a short taste questionnaire used to fetch extra suggestions.
type QuizRequest struct {
	Lane        string   `json:"lane"`
	Moods       []string `json:"moods"`
	Tone        string   `json:"tone"`
	ReadingType string   `json:"reading_type"`
	AdultOrient []string `json:"adult_orient"`
	AdultVibe   string   `json:"adult_vibe"`
}

var quizMoodGenres = map[string]string{
	"action":        "Action",
	"adventure":     "Adventure",
	"romance":       "Romance",
	"comedy":        "Comedy",
	"drama":         "Drama",
	"slice_of_life": "Slice of Life",
	"fantasy":       "Fantasy",
	"mystery":       "Mystery",
	"horror":        "Horror",
	"sports":        "Sports",
	"supernatural":  "Supernatural",
	"sci_fi":        "Sci-Fi",
	"psychological": "Psychological",
}

type quizFilters struct {
	Adult         bool
	Genres        []string
	Tags          []string
	ReadingTypes  []string
	Orient        catalog.AdultOrientationFilter
	HasConstraint bool
}

func normalizeQuiz(q QuizRequest) QuizRequest {
	lane := strings.ToLower(strings.TrimSpace(q.Lane))
	if lane != quizLaneAdult {
		lane = quizLaneSFW
	}
	var moods []string
	seen := map[string]struct{}{}
	for _, raw := range q.Moods {
		id := strings.ToLower(strings.TrimSpace(raw))
		if _, ok := quizMoodGenres[id]; !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		moods = append(moods, id)
		if len(moods) >= 4 {
			break
		}
	}
	tone := strings.ToLower(strings.TrimSpace(q.Tone))
	switch tone {
	case "light", "dark", "emotional":
	default:
		tone = ""
	}
	rt := strings.TrimSpace(q.ReadingType)
	switch rt {
	case "Webtoon", "Manga", "Light Novel":
	default:
		rt = ""
	}
	vibe := strings.ToLower(strings.TrimSpace(q.AdultVibe))
	switch vibe {
	case "romantic", "explicit":
	default:
		vibe = ""
	}
	orient := catalog.FilterValidAdultOrientations(q.AdultOrient)
	if lane != quizLaneAdult {
		orient = nil
		vibe = ""
	}
	return QuizRequest{
		Lane:        lane,
		Moods:       moods,
		Tone:        tone,
		ReadingType: rt,
		AdultOrient: orient,
		AdultVibe:   vibe,
	}
}

func QuizReady(q QuizRequest) bool {
	return quizFiltersFrom(q).HasConstraint
}

func quizFiltersFrom(q QuizRequest) quizFilters {
	q = normalizeQuiz(q)
	adult := q.Lane == quizLaneAdult
	var genres []string
	addGenre := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || !catalog.IsValidAnilistGenre(name) {
			return
		}
		for _, g := range genres {
			if g == name {
				return
			}
		}
		if len(genres) >= 3 {
			return
		}
		genres = append(genres, name)
	}
	for _, mood := range q.Moods {
		addGenre(quizMoodGenres[mood])
	}
	switch q.Tone {
	case "light":
		addGenre("Comedy")
		addGenre("Slice of Life")
	case "dark":
		addGenre("Suspense")
		addGenre("Horror")
	case "emotional":
		addGenre("Drama")
	}
	if adult && q.AdultVibe == "romantic" {
		addGenre("Romance")
	}
	var tags []string
	if adult && q.AdultVibe == "explicit" {
		tags = []string{quizTagSmut}
	}
	var types []string
	if q.ReadingType != "" {
		types = []string{q.ReadingType}
	}
	orient := catalog.AdultOrientationFilter{}
	if adult {
		orient = catalog.ResolveAdultOrientationFilter(q.AdultOrient)
	}
	has := len(genres) > 0 || len(tags) > 0 || len(types) > 0 || len(q.AdultOrient) > 0 || q.Tone != "" || q.AdultVibe != ""
	return quizFilters{
		Adult:         adult,
		Genres:        genres,
		Tags:          tags,
		ReadingTypes:  types,
		Orient:        orient,
		HasConstraint: has,
	}
}

func quizCandidateScore(r catalog.AnilistResult, f quizFilters) float64 {
	var score float64
	genreSet := map[string]struct{}{}
	for _, g := range f.Genres {
		genreSet[g] = struct{}{}
	}
	for _, g := range r.Genres {
		if _, ok := genreSet[strings.TrimSpace(g)]; ok {
			score += 1.2
		}
	}
	if len(f.Tags) > 0 {
		tagSet := map[string]struct{}{}
		for _, t := range f.Tags {
			tagSet[strings.ToLower(t)] = struct{}{}
		}
		for _, t := range r.Tags {
			if _, ok := tagSet[strings.ToLower(strings.TrimSpace(t))]; ok {
				score += 1.0
			}
		}
	}
	score += readingTypeBoost(r.ReadingType, f.ReadingTypes)
	if r.IsAdult == f.Adult {
		score += 0.2
	}
	return score
}

func quizBrowsePlans(f quizFilters) []browsePlan {
	var plans []browsePlan
	if len(f.Genres) > 0 {
		plans = append(plans, browsePlan{genreIn: f.Genres, sort: "SCORE_DESC"})
	}
	if len(f.Tags) == 1 {
		plans = append(plans, browsePlan{tagIn: f.Tags, sort: "SCORE_DESC"})
	}
	if len(f.Orient.TagIn) == 1 {
		plans = append(plans, browsePlan{tagIn: f.Orient.TagIn, sort: "SCORE_DESC"})
	}
	if len(f.Genres) > 0 {
		plans = append(plans, browsePlan{genreIn: f.Genres, sort: "POPULARITY_DESC"})
	}
	if len(plans) == 0 {
		plans = append(plans, browsePlan{sort: "SCORE_DESC"})
	}
	return plans
}

// ForQuiz returns extra suggestions for one lane from a questionnaire, excluding owned/dismissed titles.
func ForQuiz(db *database.Conn, userID int64, q QuizRequest, cfg ForUserConfig) ([]Suggestion, error) {
	q = normalizeQuiz(q)
	f := quizFiltersFrom(q)
	if !f.HasConstraint {
		return []Suggestion{}, nil
	}
	browseFn := resolveBrowse(cfg)
	blocklist, _ := catalog.LoadUserBlocklist(db, userID)
	mediaFilter := catalog.MergeBlocklistFilter(blocklist, f.Orient)

	works, err := LoadUserAnilistWorks(db, userID)
	if err != nil {
		return nil, err
	}
	seen := CollectKnownAnilistIDs(works)
	for id := range cfg.DismissedIDs {
		seen[id] = struct{}{}
	}

	var pool []rankedCandidate
	for _, plan := range quizBrowsePlans(f) {
		if len(pool) >= browsePoolSize {
			break
		}
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
			ReadingTypesIn: f.ReadingTypes,
			IsAdult:        boolPtr(f.Adult),
		})
		if err != nil {
			if len(pool) == 0 {
				return nil, err
			}
			break
		}
		for _, r := range browse {
			if r.IsAdult != f.Adult {
				continue
			}
			if _, dup := seen[r.ID]; dup {
				continue
			}
			seen[r.ID] = struct{}{}
			mg := intersectOrdered(r.Genres, f.Genres)
			mt := intersectOrdered(r.Tags, f.Tags)
			pool = append(pool, rankedCandidate{
				suggestion: Suggestion{
					Source:        "quiz",
					AnilistID:     r.ID,
					Title:         r.Title,
					ReadingType:   r.ReadingType,
					ImageURL:      r.ImageURL,
					IsAdult:       r.IsAdult,
					MatchedGenres: mg,
					MatchedTags:   mt,
				},
				score: quizCandidateScore(r, f),
			})
		}
	}
	return diversify(pool, quizCap), nil
}
