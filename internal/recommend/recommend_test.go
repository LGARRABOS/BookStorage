package recommend

import (
	"math"
	"testing"

	"bookstorage/internal/catalog"
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
