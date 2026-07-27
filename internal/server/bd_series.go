package server

import (
	"database/sql"
	"sort"
	"strings"
)

// bdSeriesCard is one series on the BD dashboard (one or more albums).
type bdSeriesCard struct {
	Name       string
	AlbumCount int
	MaxTome    int
	ImagePath  sql.NullString
	BdType     sql.NullString
	Status     sql.NullString
	IsAdult    bool
	Albums     []bdWorkRow
	// TitlesBlob / StatusesBlob help client-side search & status filters.
	TitlesBlob   string
	StatusesBlob string
}

func bdSplitSeriesTitle(title string) (series, album string) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", ""
	}
	for _, sep := range []string{" — ", " – ", " - "} {
		if i := strings.Index(title, sep); i > 0 {
			series = strings.TrimSpace(title[:i])
			album = strings.TrimSpace(title[i+len(sep):])
			if series != "" && album != "" {
				return series, album
			}
		}
	}
	return title, title
}

func bdAlbumTitle(title string) string {
	_, album := bdSplitSeriesTitle(title)
	return album
}

func groupBdWorksBySeries(works []bdWorkRow) []bdSeriesCard {
	type bucket struct {
		card bdSeriesCard
	}
	order := make([]string, 0)
	byKey := map[string]*bucket{}

	for _, w := range works {
		series, _ := bdSplitSeriesTitle(w.Title)
		if series == "" {
			continue
		}
		key := strings.ToLower(series)
		b, ok := byKey[key]
		if !ok {
			b = &bucket{card: bdSeriesCard{Name: series}}
			byKey[key] = b
			order = append(order, key)
		}
		b.card.Albums = append(b.card.Albums, w)
	}

	out := make([]bdSeriesCard, 0, len(order))
	for _, key := range order {
		card := byKey[key].card
		sort.SliceStable(card.Albums, func(i, j int) bool {
			ti, tj := card.Albums[i].Tome, card.Albums[j].Tome
			if ti == 0 && tj != 0 {
				return false
			}
			if tj == 0 && ti != 0 {
				return true
			}
			if ti != tj {
				return ti < tj
			}
			return strings.ToLower(card.Albums[i].Title) < strings.ToLower(card.Albums[j].Title)
		})
		card.AlbumCount = len(card.Albums)
		titles := make([]string, 0, len(card.Albums)+1)
		titles = append(titles, card.Name)
		statusSet := map[string]struct{}{}
		var statuses []string
		adult := false
		for _, al := range card.Albums {
			titles = append(titles, al.Title)
			if al.Tome > card.MaxTome {
				card.MaxTome = al.Tome
			}
			if al.IsAdult.Valid && al.IsAdult.Int64 == 1 {
				adult = true
			}
			if !card.ImagePath.Valid || card.ImagePath.String == "" {
				if al.ImagePath.Valid && strings.TrimSpace(al.ImagePath.String) != "" {
					card.ImagePath = al.ImagePath
				}
			}
			if !card.BdType.Valid && al.BdType.Valid {
				card.BdType = al.BdType
			}
			if al.Status.Valid {
				st := strings.TrimSpace(al.Status.String)
				if st != "" {
					if _, seen := statusSet[st]; !seen {
						statusSet[st] = struct{}{}
						statuses = append(statuses, st)
					}
				}
			}
		}
		card.IsAdult = adult
		if len(statuses) == 1 {
			card.Status = sql.NullString{String: statuses[0], Valid: true}
		}
		card.TitlesBlob = strings.ToLower(strings.Join(titles, " "))
		card.StatusesBlob = strings.Join(statuses, "|")
		out = append(out, card)
	}
	return out
}

func sortBdSeriesCards(cards []bdSeriesCard, sortBy string) {
	sort.SliceStable(cards, func(i, j int) bool {
		a, b := cards[i], cards[j]
		switch sortBy {
		case "title_desc":
			return strings.ToLower(a.Name) > strings.ToLower(b.Name)
		case "status":
			as, bs := "", ""
			if a.Status.Valid {
				as = a.Status.String
			}
			if b.Status.Valid {
				bs = b.Status.String
			}
			if as != bs {
				return as < bs
			}
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "type":
			at, bt := "", ""
			if a.BdType.Valid {
				at = a.BdType.String
			}
			if b.BdType.Valid {
				bt = b.BdType.String
			}
			if at != bt {
				return at < bt
			}
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "recent":
			return maxBdAlbumID(a.Albums) > maxBdAlbumID(b.Albums)
		case "oldest":
			return minBdAlbumID(a.Albums) < minBdAlbumID(b.Albums)
		case "modified_desc", "modified":
			return maxBdAlbumUpdated(a.Albums) > maxBdAlbumUpdated(b.Albums)
		case "modified_asc":
			return maxBdAlbumUpdated(a.Albums) < maxBdAlbumUpdated(b.Albums)
		default:
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
	})
}

func maxBdAlbumID(albums []bdWorkRow) int {
	m := 0
	for _, a := range albums {
		if a.ID > m {
			m = a.ID
		}
	}
	return m
}

func minBdAlbumID(albums []bdWorkRow) int {
	if len(albums) == 0 {
		return 0
	}
	m := albums[0].ID
	for _, a := range albums[1:] {
		if a.ID < m {
			m = a.ID
		}
	}
	return m
}

func maxBdAlbumUpdated(albums []bdWorkRow) string {
	best := ""
	for _, a := range albums {
		if !a.UpdatedAt.Valid {
			continue
		}
		s := a.UpdatedAt.String
		if s > best {
			best = s
		}
	}
	return best
}

func findBdSeriesCard(cards []bdSeriesCard, name string) (bdSeriesCard, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return bdSeriesCard{}, false
	}
	for _, c := range cards {
		if strings.ToLower(c.Name) == want {
			return c, true
		}
	}
	return bdSeriesCard{}, false
}
