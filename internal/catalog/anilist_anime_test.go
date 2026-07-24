package catalog

import "testing"

func TestMapAnilistAnimeType(t *testing.T) {
	cases := map[string]string{
		"TV":       "TV",
		"tv":       "TV",
		"TV_SHORT": "TV",
		"MOVIE":    "Film",
		"OVA":      "OVA",
		"ONA":      "ONA",
		"SPECIAL":  "Spécial",
		"MUSIC":    "Spécial",
		"":         "TV",
		"UNKNOWN":  "TV",
	}
	for in, want := range cases {
		if got := mapAnilistAnimeType(in); got != want {
			t.Errorf("mapAnilistAnimeType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAnilistAnimeResultFromMedia(t *testing.T) {
	eps := 12
	year := 2024
	m := anilistAnimeMedia{
		ID:         100,
		Format:     "TV",
		Episodes:   &eps,
		Season:     "FALL",
		SeasonYear: &year,
		IsAdult:    false,
	}
	m.Title.Romaji = "Romaji Title"
	m.Title.English = "English Title"
	m.CoverImage.Large = "http://img"

	r := anilistAnimeResultFromMedia(m)
	if r.Title != "English Title" {
		t.Errorf("title = %q, want English Title", r.Title)
	}
	if r.Episodes != 12 {
		t.Errorf("episodes = %d, want 12", r.Episodes)
	}
	if r.AnimeType != "TV" {
		t.Errorf("animeType = %q, want TV", r.AnimeType)
	}
	if r.SeasonYear != 2024 {
		t.Errorf("seasonYear = %d, want 2024", r.SeasonYear)
	}
	if r.ImageURL != "http://img" {
		t.Errorf("imageURL = %q", r.ImageURL)
	}
}
