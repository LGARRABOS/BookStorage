package server

import (
	"database/sql"
	"html/template"
	"net/url"
	"sort"
	"strings"
)

// mangaPhysDashboardURL builds /manga-phys/dashboard query links for templates.
func mangaPhysDashboardURL(sortBy, adult, series string) template.URL {
	q := url.Values{}
	sortBy = strings.TrimSpace(sortBy)
	if sortBy != "" && sortBy != "title" {
		q.Set("sort", sortBy)
	}
	if strings.TrimSpace(adult) == "only" {
		q.Set("adult", "only")
	}
	if s := strings.TrimSpace(series); s != "" {
		q.Set("series", s)
	}
	out := pathMangaPhysDashboard
	if enc := q.Encode(); enc != "" {
		out += "?" + enc
	}
	return template.URL(out)
}

// mangaPhysSeriesCard is one series on the manga phys dashboard (one or more volumes).
type mangaPhysSeriesCard struct {
	Name       string
	AlbumCount int
	MaxTome    int
	ImagePath  sql.NullString
	MangaType  sql.NullString
	Status     sql.NullString
	IsAdult    bool
	Albums     []mangaPhysWorkRow
	// TitlesBlob / StatusesBlob help client-side search & status filters.
	TitlesBlob   string
	StatusesBlob string
}

func mangaPhysSplitSeriesTitle(title string) (series, album string) {
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

func groupMangaPhysWorksBySeries(works []mangaPhysWorkRow) []mangaPhysSeriesCard {
	type bucket struct {
		card mangaPhysSeriesCard
	}
	order := make([]string, 0)
	byKey := map[string]*bucket{}

	for _, w := range works {
		series, _ := mangaPhysSplitSeriesTitle(w.Title)
		if series == "" {
			continue
		}
		key := strings.ToLower(series)
		b, ok := byKey[key]
		if !ok {
			b = &bucket{card: mangaPhysSeriesCard{Name: series}}
			byKey[key] = b
			order = append(order, key)
		}
		b.card.Albums = append(b.card.Albums, w)
	}

	out := make([]mangaPhysSeriesCard, 0, len(order))
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
			if !card.MangaType.Valid && al.MangaType.Valid {
				card.MangaType = al.MangaType
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

func sortMangaPhysSeriesCards(cards []mangaPhysSeriesCard, sortBy string) {
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
			if a.MangaType.Valid {
				at = a.MangaType.String
			}
			if b.MangaType.Valid {
				bt = b.MangaType.String
			}
			if at != bt {
				return at < bt
			}
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "recent":
			return maxMangaPhysAlbumID(a.Albums) > maxMangaPhysAlbumID(b.Albums)
		case "oldest":
			return minMangaPhysAlbumID(a.Albums) < minMangaPhysAlbumID(b.Albums)
		case "modified_desc", "modified":
			return maxMangaPhysAlbumUpdated(a.Albums) > maxMangaPhysAlbumUpdated(b.Albums)
		case "modified_asc":
			return maxMangaPhysAlbumUpdated(a.Albums) < maxMangaPhysAlbumUpdated(b.Albums)
		default:
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
	})
}

func maxMangaPhysAlbumID(albums []mangaPhysWorkRow) int {
	m := 0
	for _, a := range albums {
		if a.ID > m {
			m = a.ID
		}
	}
	return m
}

func minMangaPhysAlbumID(albums []mangaPhysWorkRow) int {
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

func maxMangaPhysAlbumUpdated(albums []mangaPhysWorkRow) string {
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

func findMangaPhysSeriesCard(cards []mangaPhysSeriesCard, name string) (mangaPhysSeriesCard, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return mangaPhysSeriesCard{}, false
	}
	for _, c := range cards {
		if strings.ToLower(c.Name) == want {
			return c, true
		}
	}
	return mangaPhysSeriesCard{}, false
}
