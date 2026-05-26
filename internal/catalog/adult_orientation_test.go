package catalog

import "testing"

func TestFilterValidAdultOrientations(t *testing.T) {
	got := FilterValidAdultOrientations([]string{"boys_love", "gay", "nope", " yuri "})
	want := []string{"boys_love", "yuri"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestLabelsMatchAny_apostrophe(t *testing.T) {
	if !labelsMatchAny([]string{"Boys' Love"}, []string{"Boys Love"}) {
		t.Fatal("expected apostrophe-insensitive match")
	}
	if labelsMatchAny([]string{"Drama", "Romance"}, boysLoveLabels) {
		t.Fatal("drama/romance alone should not match BL")
	}
	if !labelsMatchAny([]string{"Drama", "Boys' Love"}, boysLoveLabels) {
		t.Fatal("expected Boys' Love tag to match BL filter")
	}
}

func TestResolveAdultOrientationFilter_boysLoveOnly(t *testing.T) {
	f := ResolveAdultOrientationFilter([]string{AdultOrientBoysLove})
	if len(f.TagIn) != 1 || f.TagIn[0] != anilistTagBoysLove {
		t.Fatalf("TagIn=%v", f.TagIn)
	}
	if f.MatchMedia == nil || !f.MatchMedia([]string{"Drama"}, []string{"Boys' Love"}) {
		t.Fatal("expected BL media to match")
	}
	if f.MatchMedia(nil, []string{"Yuri"}) {
		t.Fatal("yuri-only should not match boys_love filter")
	}
}

func TestResolveAdultOrientationFilter_heterosexualOnly(t *testing.T) {
	f := ResolveAdultOrientationFilter([]string{AdultOrientHeterosexual})
	if len(f.TagNotIn) == 0 || f.MatchMedia == nil {
		t.Fatalf("expected exclusion filter, got %+v", f)
	}
	if f.MatchMedia([]string{"Drama"}, []string{"Boys' Love"}) {
		t.Fatal("BL must be excluded from heterosexual filter")
	}
	if !f.MatchMedia([]string{"Drama", "Romance"}, []string{"Heterosexual"}) {
		t.Fatal("heterosexual-tagged media should pass")
	}
	if !f.MatchMedia([]string{"Drama"}, []string{"Nudity"}) {
		t.Fatal("untagged non-BL media should pass heterosexual filter")
	}
}

func TestResolveAdultOrientationFilter_heterosexualAndBoysLove(t *testing.T) {
	f := ResolveAdultOrientationFilter([]string{AdultOrientHeterosexual, AdultOrientBoysLove})
	if f.MatchMedia == nil {
		t.Fatal("expected post-filter")
	}
	if !f.MatchMedia(nil, []string{"Boys' Love"}) {
		t.Fatal("BL should match when boys_love selected")
	}
	if !f.MatchMedia([]string{"Drama"}, []string{"Nudity"}) {
		t.Fatal("non-BL should match when heterosexual selected")
	}
}
