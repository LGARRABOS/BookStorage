package server

import (
	"database/sql"
	"testing"
)

func TestBdSplitSeriesTitle(t *testing.T) {
	s, a := bdSplitSeriesTitle("Aldébaran — La blonde")
	if s != "Aldébaran" || a != "La blonde" {
		t.Fatalf("got series=%q album=%q", s, a)
	}
	s, a = bdSplitSeriesTitle("Tintin")
	if s != "Tintin" || a != "Tintin" {
		t.Fatalf("standalone got series=%q album=%q", s, a)
	}
}

func TestGroupBdWorksBySeries(t *testing.T) {
	works := []bdWorkRow{
		{ID: 2, Title: "Aldébaran — La blonde", Tome: 2, Status: sql.NullString{String: "Terminé", Valid: true}},
		{ID: 1, Title: "Aldébaran — La catastrophe", Tome: 1, Status: sql.NullString{String: "Terminé", Valid: true}, ImagePath: sql.NullString{String: "https://cdn/t1.jpg", Valid: true}},
		{ID: 3, Title: "Tintin", Tome: 0},
	}
	cards := groupBdWorksBySeries(works)
	if len(cards) != 2 {
		t.Fatalf("len=%d cards=%+v", len(cards), cards)
	}
	sortBdSeriesCards(cards, "title")
	if cards[0].Name != "Aldébaran" || cards[0].AlbumCount != 2 {
		t.Fatalf("first=%+v", cards[0])
	}
	if cards[0].Albums[0].Tome != 1 || !cards[0].ImagePath.Valid {
		t.Fatalf("albums/cover=%+v img=%v", cards[0].Albums, cards[0].ImagePath)
	}
	if cards[1].Name != "Tintin" || cards[1].AlbumCount != 1 {
		t.Fatalf("second=%+v", cards[1])
	}
}
