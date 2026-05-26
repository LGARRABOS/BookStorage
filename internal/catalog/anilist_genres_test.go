package catalog

import "testing"

func TestAnilistGenres_nonEmptyUnique(t *testing.T) {
	genres := AnilistGenres()
	if len(genres) == 0 {
		t.Fatal("expected non-empty genre list")
	}
	seen := make(map[string]struct{}, len(genres))
	for _, g := range genres {
		if g == "" {
			t.Fatal("empty genre name")
		}
		if _, dup := seen[g]; dup {
			t.Fatalf("duplicate genre: %q", g)
		}
		seen[g] = struct{}{}
	}
}

func TestFilterValidAnilistGenres(t *testing.T) {
	got := FilterValidAnilistGenres([]string{"Action", "bogus", "Action", "Romance", "Sci-Fi", "Drama"}, 3)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3: %#v", len(got), got)
	}
	if got[0] != "Action" || got[1] != "Romance" || got[2] != "Sci-Fi" {
		t.Fatalf("unexpected order: %#v", got)
	}
	if len(FilterValidAnilistGenres([]string{"nope"}, 3)) != 0 {
		t.Fatal("expected empty for invalid only")
	}
}
