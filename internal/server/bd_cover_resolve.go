package server

import (
	"strconv"
	"strings"
	"unicode"

	"bookstorage/internal/catalog"
)

// resolveBdCoverURL looks up a cover via BnF / Google Books / Open Library.
// Overridable in tests. tome helps disambiguate volumes of the same series.
var resolveBdCoverURL = resolveBdCoverURLDefault

type bdCoverResolve struct {
	URL     string
	IsAdult *bool
}

func resolveFromOpenLibraryBD(res *catalog.OpenLibraryBdResult) bdCoverResolve {
	if res == nil {
		return bdCoverResolve{}
	}
	return bdCoverResolve{URL: strings.TrimSpace(res.ImageURL), IsAdult: adultBoolPtr(res.IsAdult)}
}

// foldCoverKey normalizes titles for cover matching (case, accents, punctuation).
func foldCoverKey(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		switch r {
		case 'à', 'á', 'â', 'ä', 'ã', 'å':
			b.WriteByte('a')
		case 'è', 'é', 'ê', 'ë':
			b.WriteByte('e')
		case 'ì', 'í', 'î', 'ï':
			b.WriteByte('i')
		case 'ò', 'ó', 'ô', 'ö', 'õ':
			b.WriteByte('o')
		case 'ù', 'ú', 'û', 'ü':
			b.WriteByte('u')
		case 'ý', 'ÿ':
			b.WriteByte('y')
		case 'ç':
			b.WriteByte('c')
		case 'ñ':
			b.WriteByte('n')
		case 'æ':
			b.WriteString("ae")
		case 'œ':
			b.WriteString("oe")
		default:
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			} else if unicode.IsSpace(r) || r == '-' || r == '—' || r == '–' || r == '\'' || r == '’' {
				b.WriteByte(' ')
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// bdCoverSearchTitles returns safe title queries for cover lookup.
// Never searches by series name alone (that maps every tome to the same popular cover).
func bdCoverSearchTitles(title string) []string {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	seen := map[string]struct{}{title: {}}
	out := []string{title}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	series, album := bdSplitSeriesTitle(title)
	if album != "" && !strings.EqualFold(album, series) {
		add(series + " " + album)
		add(`"` + album + `" ` + series)
	}
	return out
}

// scoreOpenLibraryCoverMatch ranks a candidate title against the wanted album.
// Returns -1 when the hit is clearly the wrong volume (e.g. same series, other subtitle).
func scoreOpenLibraryCoverMatch(candidateTitle, fullTitle string, tome int) int {
	cand := foldCoverKey(candidateTitle)
	if cand == "" {
		return -1
	}
	series, album := bdSplitSeriesTitle(fullTitle)
	seriesKey := foldCoverKey(series)
	albumKey := foldCoverKey(album)
	fullKey := foldCoverKey(fullTitle)

	score := 0
	if albumKey != "" && albumKey != seriesKey {
		if strings.Contains(cand, albumKey) {
			score += 100
		} else {
			// Candidate does not mention the album subtitle → wrong tome / wrong work.
			return -1
		}
		if seriesKey != "" && strings.Contains(cand, seriesKey) {
			score += 25
		}
	} else {
		if fullKey != "" && (strings.Contains(cand, fullKey) || strings.Contains(fullKey, cand)) {
			score += 60
		} else if seriesKey != "" && strings.Contains(cand, seriesKey) {
			score += 20
		} else {
			return -1
		}
	}
	if tome > 0 {
		tomeStr := strconv.Itoa(tome)
		for _, needle := range []string{
			"tome " + tomeStr,
			"t " + tomeStr,
			"vol " + tomeStr,
			"volume " + tomeStr,
			"#" + tomeStr,
		} {
			if strings.Contains(cand, needle) {
				score += 15
				break
			}
		}
	}
	return score
}

func pickOpenLibraryCover(results []catalog.OpenLibraryBdResult, fullTitle string, tome int) *catalog.OpenLibraryBdResult {
	bestScore := -1
	var best *catalog.OpenLibraryBdResult
	for i := range results {
		if strings.TrimSpace(results[i].ImageURL) == "" {
			continue
		}
		sc := scoreOpenLibraryCoverMatch(results[i].Title, fullTitle, tome)
		if sc > bestScore {
			bestScore = sc
			best = &results[i]
		}
	}
	if bestScore < 0 {
		return nil
	}
	return best
}

func firstCoverURL(fns ...func() (string, error)) (string, error) {
	var lastErr error
	for _, fn := range fns {
		u, err := fn()
		if err != nil {
			// Only Open Library search rate-limits pause the job; other sources soft-skip upstream.
			if catalog.IsOpenLibraryRateLimit(err) {
				return "", err
			}
			lastErr = err
			continue
		}
		if strings.TrimSpace(u) != "" {
			return strings.TrimSpace(u), nil
		}
	}
	return "", lastErr
}

func resolveBdCoverURLDefault(source, externalID, title, isbn string, tome int) (bdCoverResolve, error) {
	source = strings.ToLower(strings.TrimSpace(source))
	ext := strings.TrimSpace(externalID)
	isbn = strings.TrimSpace(isbn)
	title = strings.TrimSpace(title)

	if isbn != "" {
		u, err := firstCoverURL(
			func() (string, error) { return catalog.LookupBnFCoverByISBN(isbn) },
			func() (string, error) { return catalog.LookupGoogleBooksCoverByISBN(isbn) },
			func() (string, error) { return catalog.OpenLibraryCoverByISBN(isbn) },
		)
		if err != nil {
			return bdCoverResolve{}, err
		}
		if u != "" {
			return bdCoverResolve{URL: u}, nil
		}
	}

	// Only treat as Open Library id when source says so, or the key looks like /works/OL…W.
	if ext != "" && (source == "openlibrary" || source == "ol" || strings.HasPrefix(ext, "/works/")) {
		res, err := catalog.GetOpenLibraryBDByID(ext)
		if err != nil {
			if catalog.IsOpenLibraryRateLimit(err) {
				return bdCoverResolve{}, err
			}
			// Fall through to title search when the key is unknown.
		} else if res != nil && strings.TrimSpace(res.ImageURL) != "" {
			return resolveFromOpenLibraryBD(res), nil
		}
	}

	var lastErr error
	for _, q := range bdCoverSearchTitles(title) {
		results, err := catalog.SearchOpenLibraryBDCover(q, 8)
		if err != nil {
			lastErr = err
			if catalog.IsOpenLibraryRateLimit(err) {
				return bdCoverResolve{}, err
			}
			continue
		}
		if best := pickOpenLibraryCover(results, title, tome); best != nil {
			return resolveFromOpenLibraryBD(best), nil
		}
	}
	// Intentionally skip Google Books title search: without result scoring it often
	// attaches another volume's cover of the same series.
	if lastErr != nil {
		return bdCoverResolve{}, lastErr
	}
	return bdCoverResolve{}, nil
}
