//go:build integration

package catalog

import (
	"testing"
)

func TestGetMediaByID_live(t *testing.T) {
	d, err := GetMediaByID(30013) // One Piece - stable id
	if err != nil {
		t.Fatal(err)
	}
	if d == nil || d.ID != 30013 {
		t.Fatalf("unexpected media: %+v", d)
	}
}

func TestBrowseMedia_live(t *testing.T) {
	r, err := BrowseMedia(BrowseMediaParams{
		GenreIn:    []string{"Action"},
		PerPage:    5,
		MaxResults: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r) == 0 {
		t.Fatal("expected browse results")
	}
}
