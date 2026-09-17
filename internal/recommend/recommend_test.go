package recommend

import (
	"math"
	"path/filepath"
	"testing"

	"bookstorage/internal/catalog"
	"bookstorage/internal/config"
	"bookstorage/internal/database"
)

func TestCollectKnownAnilistIDs(t *testing.T) {
	works := []userWork{
		{AnilistID: " 42 ", Rating: 5, Status: "En cours"},
		{AnilistID: "bad", Rating: 0, Status: ""},
		{AnilistID: "100", Rating: 3, Status: "Terminé"},
	}
	got := CollectKnownAnilistIDs(works)
	if len(got) != 2 {
		t.Fatalf("want 2 ids, got %d: %#v", len(got), got)
	}
	if _, ok := got[42]; !ok {
		t.Error("expected id 42")
	}
	if _, ok := got[100]; !ok {
		t.Error("expected id 100")
	}
}

func TestAggregateProfile(t *testing.T) {
	details := []*catalog.MediaDetail{
		{
			Genres: []string{"Action", "Drama"},
			Tags: []catalog.MediaTag{
				{Name: "Shounen", Rank: 80},
				{Name: "Military", Rank: 40},
			},
		},
		{
			Genres: []string{"Action"},
			Tags:   []catalog.MediaTag{{Name: "Shounen", Rank: 100}},
		},
	}
	weights := []float64{1.0, 2.0}
	tp := aggregateProfile(details, weights)
	if len(tp.Genres) < 1 || tp.Genres[0].Name != "Action" {
		t.Errorf("top genre should be Action, got %#v", tp.Genres)
	}
	// Action: 1 + 2 = 3; Drama: 1
	if math.Abs(tp.Genres[0].Score-3.0) > 1e-6 {
		t.Errorf("Action score: got %v want 3", tp.Genres[0].Score)
	}
}

func TestIntersectOrdered(t *testing.T) {
	got := intersectOrdered([]string{"Romance", "Drama", "X"}, []string{"Drama", "Romance"})
	if len(got) != 2 || got[0] != "Romance" || got[1] != "Drama" {
		t.Errorf("got %v", got)
	}
	if len(intersectOrdered([]string{"A"}, []string{"B"})) != 0 {
		t.Error("expected empty intersection")
	}
}

func TestBuildWeightedListOrdering(t *testing.T) {
	o := DefaultOptions()
	works := []userWork{
		{AnilistID: "2", Rating: 5, Status: "Terminé"},
		{AnilistID: "1", Rating: 5, Status: "Terminé"},
	}
	list := buildWeightedList(works, o)
	if len(list) != 2 {
		t.Fatalf("len=%d", len(list))
	}
	// Same weight → tie-break by id ascending
	if list[0].id != 1 || list[1].id != 2 {
		t.Errorf("order: got [%d,%d] want [1,2]", list[0].id, list[1].id)
	}
}

func TestProfileOverlapScore(t *testing.T) {
	p := TasteProfile{
		Genres: []scoredGenre{{Name: "Action", Score: 3}, {Name: "Drama", Score: 1}},
		Tags:   []scoredTag{{Name: "Shounen", Score: 2}},
	}
	m := buildProfileMaps(p)
	got := profileOverlapScore(m, []string{"Action", "Comedy"}, []string{"Shounen"})
	want := 3.0 + 2.0*0.85
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("overlap: got %v want %v", got, want)
	}
}

func TestCandidateScoreRecommendationBeatsBrowse(t *testing.T) {
	overlap := 2.0
	browse := candidateScore(overlap, "browse", 0, 0)
	rec := candidateScore(overlap, "recommendation", 85, 2.0)
	if rec <= browse {
		t.Fatalf("recommendation should outrank browse at equal overlap: browse=%v rec=%v", browse, rec)
	}
}

func TestRankCandidatesOrdersByScore(t *testing.T) {
	ranked := rankCandidates([]rankedCandidate{
		{suggestion: Suggestion{AnilistID: 3}, score: 1.5},
		{suggestion: Suggestion{AnilistID: 1}, score: 4.0},
		{suggestion: Suggestion{AnilistID: 2}, score: 4.0},
	})
	if len(ranked) != 3 || ranked[0].AnilistID != 1 || ranked[1].AnilistID != 2 || ranked[2].AnilistID != 3 {
		t.Fatalf("order: %#v", ranked)
	}
}

func TestParseJSONStringList(t *testing.T) {
	got := parseJSONStringList(`["Action"," Drama ", "Action"]`)
	if len(got) != 2 || got[0] != "Action" || got[1] != "Drama" {
		t.Fatalf("got %v", got)
	}
	if parseJSONStringList("") != nil {
		t.Fatal("empty should be nil")
	}
}

func TestSplitTaste_dropsLowRating(t *testing.T) {
	o := DefaultOptions()
	list := buildWeightedList([]userWork{
		{AnilistID: "1", Rating: 5, Status: "Terminé"},
		{AnilistID: "2", Rating: 1, Status: "Terminé"},
		{AnilistID: "3", Rating: 5, Status: "Abandonné"},
	}, o)
	liked, disliked := splitTaste(list)
	if len(liked) != 1 || liked[0].id != 1 {
		t.Fatalf("liked=%#v", liked)
	}
	if len(disliked) != 2 {
		t.Fatalf("disliked=%d", len(disliked))
	}
}

func TestDefaultBrowsePlans_atMostOneTag(t *testing.T) {
	p := TasteProfile{
		Genres: []scoredGenre{{Name: "Action", Score: 4}, {Name: "Drama", Score: 2}, {Name: "Comedy", Score: 1}},
		Tags:   []scoredTag{{Name: "Shounen", Score: 3}, {Name: "School", Score: 2}, {Name: "Military", Score: 1}},
	}
	plans := defaultBrowsePlans(p)
	if len(plans) < 2 {
		t.Fatalf("want genre + tag plans, got %d", len(plans))
	}
	var sawTag bool
	for _, plan := range plans {
		if len(plan.tagIn) > 1 {
			t.Fatalf("tag_in must have at most 1 tag, got %v", plan.tagIn)
		}
		if len(plan.genreIn) > 2 {
			t.Fatalf("genre_in must have at most 2 genres, got %v", plan.genreIn)
		}
		if len(plan.tagIn) == 1 {
			sawTag = true
			if plan.tagIn[0] != "Shounen" {
				t.Fatalf("top tag: %v", plan.tagIn)
			}
		}
	}
	if !sawTag {
		t.Fatal("expected a tag-only browse plan")
	}
}

func TestDominantReadingTypes_majority(t *testing.T) {
	got := dominantReadingTypes([]weightedWork{
		{userWork: userWork{ReadingType: "Manga"}, weight: 8},
		{userWork: userWork{ReadingType: "Webtoon"}, weight: 1},
	})
	if len(got) != 1 || got[0] != "Manga" {
		t.Fatalf("got %v", got)
	}
}

func TestReadingTypeBoost(t *testing.T) {
	if readingTypeBoost("Manga", []string{"Manga"}) <= 0 {
		t.Fatal("match should boost")
	}
	if readingTypeBoost("Webtoon", []string{"Manga"}) >= 0 {
		t.Fatal("single-type mismatch should penalize")
	}
	if readingTypeBoost("Webtoon", []string{"Manga", "Webtoon"}) <= 0 {
		t.Fatal("listed type should boost")
	}
}

func TestFinalizeScore_dislikeLowers(t *testing.T) {
	base := 4.0
	if finalizeScore(base, 2.0, 0) >= base {
		t.Fatal("dislike overlap should lower the score")
	}
}

func TestDiversify_capsSameRelatedWork(t *testing.T) {
	var cands []rankedCandidate
	for i := 1; i <= 6; i++ {
		cands = append(cands, rankedCandidate{
			score: float64(10 - i),
			suggestion: Suggestion{
				AnilistID:        i,
				RelatedAnilistID: 99,
				MatchedGenres:    []string{"Action"},
			},
		})
	}
	cands = append(cands, rankedCandidate{
		score: 0.5,
		suggestion: Suggestion{
			AnilistID:     50,
			MatchedGenres: []string{"Drama"},
		},
	})
	got := diversify(cands, 4)
	related := 0
	hasDrama := false
	for _, s := range got {
		if s.RelatedAnilistID == 99 {
			related++
		}
		if s.AnilistID == 50 {
			hasDrama = true
		}
	}
	if related > maxPerRelatedWork {
		t.Fatalf("related cap: got %d in %d results", related, len(got))
	}
	if !hasDrama {
		t.Fatal("expected a non-related title to fill remaining slots")
	}
}

func TestMediaFetchTargets_alwaysIncludesTopLiked(t *testing.T) {
	liked := []weightedWork{
		{id: 1, userWork: userWork{Genres: []string{"Action"}}},
		{id: 2},
		{id: 3},
	}
	local := map[int]*catalog.MediaDetail{1: {ID: 1}}
	got := mediaFetchTargets(liked, local)
	if len(got) < 1 || got[0].id != 1 {
		t.Fatalf("top liked must be fetched for rec edges: %#v", got)
	}
	ids := map[int]bool{}
	for _, w := range got {
		ids[w.id] = true
	}
	if !ids[2] || !ids[3] {
		t.Fatalf("missing local-less works: %v", ids)
	}
}

func openRecommendTestDB(t *testing.T) *database.Conn {
	t.Helper()
	dir := t.TempDir()
	s := &config.Settings{
		Database:            filepath.Join(dir, "db.sqlite"),
		SecretKey:           "0123456789abcdef0123456789abcdef",
		Environment:         "development",
		SuperadminUsername:  "admin",
		SuperadminPassword:  "TestAdmin!99",
		DataDirectory:       dir,
		UploadFolder:        filepath.Join(dir, "img"),
		ProfileUploadFolder: filepath.Join(dir, "av"),
	}
	db, err := database.Open(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.EnsureSchema(db, s); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestForUser_localProfilePenalizesDroppedGenre(t *testing.T) {
	db := openRecommendTestDB(t)
	if _, err := db.Exec(`INSERT INTO catalog (title, reading_type, source, external_id, genres, tags) VALUES
		('Liked', 'Manga', 'anilist', '10', '["Action"]', '["Shounen"]'),
		('Dropped', 'Manga', 'anilist', '20', '["Romance"]', '[]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO works (title, chapter, user_id, status, reading_type, rating, catalog_id)
		SELECT 'Liked', 1, 1, 'Terminé', 'Manga', 5, id FROM catalog WHERE external_id = '10'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO works (title, chapter, user_id, status, reading_type, rating, catalog_id)
		SELECT 'Dropped', 1, 1, 'Abandonné', 'Manga', 1, id FROM catalog WHERE external_id = '20'`); err != nil {
		t.Fatal(err)
	}

	var browseCalls []catalog.BrowseMediaParams
	cfg := DefaultForUserConfig()
	cfg.GetMedia = func(id int) (*catalog.MediaDetail, error) {
		if id != 10 {
			t.Fatalf("unexpected GetMedia id %d", id)
		}
		return &catalog.MediaDetail{
			ID:     10,
			Title:  "Liked",
			Genres: []string{"Action"},
			Recommendations: []catalog.AnilistResult{
				{ID: 100, Title: "Action Rec", ReadingType: "Manga", Genres: []string{"Action"}},
				{ID: 101, Title: "Romance Rec", ReadingType: "Manga", Genres: []string{"Romance"}},
			},
		}, nil
	}
	cfg.Browse = func(p catalog.BrowseMediaParams) ([]catalog.AnilistResult, int, error) {
		browseCalls = append(browseCalls, p)
		if p.IsAdult != nil && *p.IsAdult {
			t.Fatal("sfw-only library should not request adult browse")
		}
		if len(p.TagIn) > 1 {
			t.Fatalf("tag_in too wide: %v", p.TagIn)
		}
		return []catalog.AnilistResult{
			{ID: 200, Title: "Action Browse", ReadingType: "Manga", Genres: []string{"Action"}},
			{ID: 201, Title: "Romance Browse", ReadingType: "Manga", Genres: []string{"Romance"}},
		}, 2, nil
	}

	out, err := ForUser(db, 1, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || len(out.Results) == 0 {
		t.Fatal("expected suggestions")
	}
	if len(browseCalls) == 0 {
		t.Fatal("expected browse")
	}
	pos := map[int]int{}
	for i, s := range out.Results {
		pos[s.AnilistID] = i
	}
	if _, ok := pos[100]; !ok {
		t.Fatalf("missing action graph rec: %#v", out.Results)
	}
	if rPos, ok := pos[101]; ok {
		if rPos < pos[100] {
			t.Fatalf("romance graph rec should not outrank action rec: %#v", out.Results)
		}
	}
	if len(out.AdultResults) != 0 {
		t.Fatalf("sfw library should not yield adult lane: %#v", out.AdultResults)
	}
}

func TestBrowseReadingTypes_locksWebtoon(t *testing.T) {
	got := browseReadingTypes([]weightedWork{
		{userWork: userWork{ReadingType: "Webtoon"}, weight: 5},
		{userWork: userWork{ReadingType: "Manga"}, weight: 6},
	})
	if len(got) != 1 || got[0] != webtoonTypeName {
		t.Fatalf("got %v", got)
	}
}

func TestReadingTypeBoost_webtoonBeatsMangaMatch(t *testing.T) {
	web := readingTypeBoost("Webtoon", []string{"Webtoon"})
	manga := readingTypeBoost("Manga", []string{"Manga"})
	if web <= manga {
		t.Fatalf("webtoon boost %v should exceed manga %v", web, manga)
	}
}

func TestForUser_separatesAdultFromSfw(t *testing.T) {
	db := openRecommendTestDB(t)
	if _, err := db.Exec(`INSERT INTO catalog (title, reading_type, source, external_id, genres, tags) VALUES
		('SFW', 'Webtoon', 'anilist', '10', '["Drama"]', '[]'),
		('Adult', 'Webtoon', 'anilist', '30', '["Romance"]', '[]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO works (title, chapter, user_id, status, reading_type, rating, is_adult, catalog_id)
		SELECT 'SFW', 1, 1, 'En cours', 'Webtoon', 5, 0, id FROM catalog WHERE external_id = '10'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO works (title, chapter, user_id, status, reading_type, rating, is_adult, catalog_id)
		SELECT 'Adult', 1, 1, 'En cours', 'Webtoon', 5, 1, id FROM catalog WHERE external_id = '30'`); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultForUserConfig()
	cfg.GetMedia = func(id int) (*catalog.MediaDetail, error) {
		switch id {
		case 10:
			return &catalog.MediaDetail{
				ID:     10,
				Title:  "SFW",
				Genres: []string{"Drama"},
				Recommendations: []catalog.AnilistResult{
					{ID: 100, Title: "SFW Rec", ReadingType: "Webtoon", Genres: []string{"Drama"}, IsAdult: false},
					{ID: 109, Title: "Adult leak", ReadingType: "Webtoon", Genres: []string{"Romance"}, IsAdult: true},
				},
			}, nil
		case 30:
			return &catalog.MediaDetail{
				ID:     30,
				Title:  "Adult",
				Genres: []string{"Romance"},
				Recommendations: []catalog.AnilistResult{
					{ID: 300, Title: "Adult Rec", ReadingType: "Webtoon", Genres: []string{"Romance"}, IsAdult: true},
				},
			}, nil
		default:
			t.Fatalf("unexpected GetMedia id %d", id)
			return nil, nil
		}
	}
	cfg.Browse = func(p catalog.BrowseMediaParams) ([]catalog.AnilistResult, int, error) {
		if len(p.ReadingTypesIn) != 1 || p.ReadingTypesIn[0] != "Webtoon" {
			t.Fatalf("expected webtoon browse, got %v", p.ReadingTypesIn)
		}
		adult := p.IsAdult != nil && *p.IsAdult
		if adult {
			return []catalog.AnilistResult{
				{ID: 400, Title: "Adult Browse", ReadingType: "Webtoon", Genres: []string{"Romance"}, IsAdult: true},
			}, 1, nil
		}
		return []catalog.AnilistResult{
			{ID: 200, Title: "SFW Browse", ReadingType: "Webtoon", Genres: []string{"Drama"}, IsAdult: false},
		}, 1, nil
	}

	out, err := ForUser(db, 1, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("nil result")
	}
	sfwIDs := map[int]bool{}
	for _, s := range out.Results {
		if s.IsAdult {
			t.Fatalf("adult title in sfw lane: %#v", s)
		}
		sfwIDs[s.AnilistID] = true
	}
	adultIDs := map[int]bool{}
	for _, s := range out.AdultResults {
		if !s.IsAdult {
			t.Fatalf("sfw title in adult lane: %#v", s)
		}
		adultIDs[s.AnilistID] = true
	}
	if !sfwIDs[100] || !sfwIDs[200] {
		t.Fatalf("missing sfw recs: %#v", out.Results)
	}
	if sfwIDs[109] || sfwIDs[300] || sfwIDs[400] {
		t.Fatalf("adult rec leaked into sfw: %#v", out.Results)
	}
	if !adultIDs[109] || !adultIDs[300] || !adultIDs[400] {
		t.Fatalf("missing adult recs: %#v", out.AdultResults)
	}
}
