package catalog

import "testing"

func TestMergeBlocklistFilter_genreBlocks(t *testing.T) {
	bl := UserBlocklist{Genres: []string{"Horror"}}
	f := MergeBlocklistFilter(bl, AdultOrientationFilter{})
	if f.MatchMedia == nil {
		t.Fatal("expected MatchMedia")
	}
	if f.MatchMedia([]string{"Horror"}, nil) {
		t.Fatal("horror genre should be blocked")
	}
	if !f.MatchMedia([]string{"Romance"}, nil) {
		t.Fatal("romance should pass")
	}
}

func TestMergeBlocklistFilter_tagNotInMerged(t *testing.T) {
	bl := UserBlocklist{Tags: []string{"Gore"}}
	orient := AdultOrientationFilter{TagNotIn: []string{"Yuri"}}
	f := MergeBlocklistFilter(bl, orient)
	if len(f.TagNotIn) != 2 {
		t.Fatalf("TagNotIn len=%d want 2", len(f.TagNotIn))
	}
}
