package recommend

import (
	"testing"

	"bookstorage/internal/catalog"
)

func TestNormalizeQuiz_stripsAdultOnSfwLane(t *testing.T) {
	q := normalizeQuiz(QuizRequest{
		Lane:        "sfw",
		Moods:       []string{"Romance", "bogus", "comedy"},
		Tone:        "light",
		ReadingType: "Webtoon",
		AdultOrient: []string{"boys_love"},
		AdultVibe:   "explicit",
	})
	if q.Lane != quizLaneSFW {
		t.Fatalf("lane=%q", q.Lane)
	}
	if len(q.Moods) != 2 || q.Moods[0] != "romance" || q.Moods[1] != "comedy" {
		t.Fatalf("moods=%v", q.Moods)
	}
	if len(q.AdultOrient) != 0 || q.AdultVibe != "" {
		t.Fatalf("adult fields leaked: %#v", q)
	}
	if q.ReadingType != "Webtoon" {
		t.Fatalf("reading_type=%q", q.ReadingType)
	}
}

func TestQuizFiltersFrom_adultExplicit(t *testing.T) {
	f := quizFiltersFrom(QuizRequest{
		Lane:      "adult",
		Moods:     []string{"romance"},
		AdultVibe: "explicit",
	})
	if !f.Adult {
		t.Fatal("expected adult lane")
	}
	if len(f.Tags) != 1 || f.Tags[0] != quizTagSmut {
		t.Fatalf("tags=%v", f.Tags)
	}
	foundRomance := false
	for _, g := range f.Genres {
		if g == "Romance" {
			foundRomance = true
		}
	}
	if !foundRomance {
		t.Fatalf("genres=%v", f.Genres)
	}
}

func TestQuizReady(t *testing.T) {
	if QuizReady(QuizRequest{}) {
		t.Fatal("empty quiz should not be ready")
	}
	if !QuizReady(QuizRequest{Moods: []string{"action"}}) {
		t.Fatal("mood should be enough")
	}
}

func TestForQuiz_respectsAdultLane(t *testing.T) {
	db := openRecommendTestDB(t)
	cfg := DefaultForUserConfig()
	cfg.Browse = func(p catalog.BrowseMediaParams) ([]catalog.AnilistResult, int, error) {
		if p.IsAdult == nil || !*p.IsAdult {
			t.Fatal("adult quiz must set isAdult=true")
		}
		return []catalog.AnilistResult{
			{ID: 1, Title: "Adult", ReadingType: "Webtoon", Genres: []string{"Romance"}, IsAdult: true},
			{ID: 2, Title: "Leak", ReadingType: "Webtoon", Genres: []string{"Romance"}, IsAdult: false},
		}, 2, nil
	}
	out, err := ForQuiz(db, 1, QuizRequest{Lane: "adult", Moods: []string{"romance"}}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].AnilistID != 1 || !out[0].IsAdult || out[0].Source != "quiz" {
		t.Fatalf("got %#v", out)
	}
}

func TestForQuiz_sfwExcludesAdultHits(t *testing.T) {
	db := openRecommendTestDB(t)
	cfg := DefaultForUserConfig()
	cfg.Browse = func(p catalog.BrowseMediaParams) ([]catalog.AnilistResult, int, error) {
		if p.IsAdult == nil || *p.IsAdult {
			t.Fatal("sfw quiz must set isAdult=false")
		}
		return []catalog.AnilistResult{
			{ID: 9, Title: "SFW", ReadingType: "Webtoon", Genres: []string{"Action"}, IsAdult: false},
			{ID: 8, Title: "Adult", ReadingType: "Webtoon", Genres: []string{"Action"}, IsAdult: true},
		}, 2, nil
	}
	out, err := ForQuiz(db, 1, QuizRequest{Lane: "sfw", Moods: []string{"action"}, ReadingType: "Webtoon"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].AnilistID != 9 || out[0].IsAdult {
		t.Fatalf("got %#v", out)
	}
}
