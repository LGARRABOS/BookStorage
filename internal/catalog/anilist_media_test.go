package catalog

import "testing"

func TestPaginateBrowseBatches(t *testing.T) {
	pages := []browsePageBatch{
		{items: makeResults(1, 3), full: true},
		{items: makeResults(4, 8), full: true},
		{items: makeResults(9, 12), full: false},
	}

	got, hasNext := paginateBrowseBatches(pages, 0, 5)
	if len(got) != 5 || got[0].ID != 1 || got[4].ID != 5 {
		t.Fatalf("page1: got %d items ids=%v", len(got), ids(got))
	}
	if !hasNext {
		t.Fatal("page1: expected hasNext")
	}

	got, hasNext = paginateBrowseBatches(pages, 5, 5)
	if len(got) != 5 || got[0].ID != 6 || got[4].ID != 10 {
		t.Fatalf("page2: got %d items ids=%v", len(got), ids(got))
	}
	if !hasNext {
		t.Fatal("page2: expected hasNext")
	}

	got, hasNext = paginateBrowseBatches(pages, 10, 5)
	if len(got) != 2 || got[0].ID != 11 || got[1].ID != 12 {
		t.Fatalf("page3: got %d items ids=%v", len(got), ids(got))
	}
	if hasNext {
		t.Fatal("page3: expected no hasNext")
	}
}

func makeResults(from, to int) []AnilistResult {
	out := make([]AnilistResult, 0, to-from+1)
	for id := from; id <= to; id++ {
		out = append(out, AnilistResult{ID: id, Title: "t"})
	}
	return out
}

func ids(r []AnilistResult) []int {
	out := make([]int, len(r))
	for i, x := range r {
		out[i] = x.ID
	}
	return out
}
