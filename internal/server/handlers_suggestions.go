package server

import (
	"net/http"

	"bookstorage/internal/catalog"
	"bookstorage/internal/i18n"
)

// suggestionLanes are the two audiences of the suggestions page. Each lane owns
// its own recommendation row and its own explore panel so all-ages and adult
// results never share a grid.
var suggestionLanes = []string{"sfw", "adult"}

// userHasAdultWorks reports whether the library already contains adult works.
// The adult explore panel stays available either way; this only drives the
// "no adult work yet" hint shown above the adult recommendation row.
func (a *App) userHasAdultWorks(userID int64) bool {
	var n int
	if err := a.DB.QueryRow(
		`SELECT COUNT(1) FROM works WHERE user_id = ? AND COALESCE(is_adult, 0) != 0`,
		userID,
	).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// HandleSuggestions renders the suggestions page: personalized recommendations
// (plus the taste quiz) and genre exploration, split per audience.
func (a *App) HandleSuggestions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	lang := a.currentLang(r)
	hasAdult := false
	if userID, ok := a.currentUserID(r); ok {
		hasAdult = a.userHasAdultWorks(int64(userID))
	}
	data := map[string]any{
		"Genres":            catalog.AnilistGenres(),
		"ReadingTypes":      catalogBrowseReadingTypes,
		"CatalogSource":     parseCatalogSource(r.URL.Query().Get("source")),
		"SuggestionLanes":   suggestionLanes,
		"HasAdultWorks":     hasAdult,
		"MobileTopbarTitle": i18n.T(lang)["suggestions.title"],
	}
	a.renderTemplate(w, r, "suggestions", a.mergeData(r, data))
}
