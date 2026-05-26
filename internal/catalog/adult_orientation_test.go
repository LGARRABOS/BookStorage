package catalog

import "testing"

func TestFilterValidAdultOrientations(t *testing.T) {
	got := FilterValidAdultOrientations([]string{"gay", "Gay", "nope", " lesbian "})
	want := []string{"gay", "lesbian"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestResolveAdultOrientationFilter_gayOnly(t *testing.T) {
	f := ResolveAdultOrientationFilter([]string{AdultOrientGay})
	if len(f.TagIn) == 0 || f.MatchFunc != nil {
		t.Fatalf("expected TagIn only, got %+v", f)
	}
	if !tagsMatchAny([]string{"Boys Love"}, f.TagIn) {
		t.Fatal("TagIn should include Boys Love")
	}
}

func TestResolveAdultOrientationFilter_heteroOnly(t *testing.T) {
	f := ResolveAdultOrientationFilter([]string{AdultOrientHetero})
	if len(f.TagNotIn) == 0 || f.MatchFunc != nil {
		t.Fatalf("expected TagNotIn for hetero-only, got %+v", f)
	}
}

func TestResolveAdultOrientationFilter_heteroAndGay(t *testing.T) {
	f := ResolveAdultOrientationFilter([]string{AdultOrientHetero, AdultOrientGay})
	if f.MatchFunc == nil {
		t.Fatal("expected post-filter")
	}
	if !f.MatchFunc([]string{"Boys Love"}) {
		t.Fatal("BL should match gay selection")
	}
	if !f.MatchFunc([]string{"Drama"}) {
		t.Fatal("no BL/GL tag should match hetero selection")
	}
	if f.MatchFunc([]string{"Girls Love"}) {
		t.Fatal("GL should not match when lesbian not selected")
	}
}
