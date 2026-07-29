package server

import (
	"net/http"
)

// HandleHub renders the central hub shown to logged-in users at the site root.
// It gathers the sections available to the current user (Manga/Webtoon module,
// profile, administration) so more modules can be added later.
func (a *App) HandleHub(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}

	var isAdmin int
	_ = a.DB.QueryRow(`SELECT is_admin FROM users WHERE id = ?`, userID).Scan(&isAdmin)

	var worksInProgress int
	_ = a.DB.QueryRow(
		`SELECT COUNT(*) FROM works WHERE user_id = ? AND status = ?`,
		userID, "En cours",
	).Scan(&worksInProgress)

	var animeInProgress int
	_ = a.DB.QueryRow(
		`SELECT COUNT(*) FROM anime_works WHERE user_id = ? AND status = ?`,
		userID, "En cours",
	).Scan(&animeInProgress)

	var bdInProgress int
	_ = a.DB.QueryRow(
		`SELECT COUNT(*) FROM bd_works WHERE user_id = ? AND status = ?`,
		userID, "En cours",
	).Scan(&bdInProgress)

	var mangaPhysInProgress int
	_ = a.DB.QueryRow(
		`SELECT COUNT(*) FROM manga_phys_works WHERE user_id = ? AND status = ?`,
		userID, "En cours",
	).Scan(&mangaPhysInProgress)

	var libraryCount int
	_ = a.DB.QueryRow(
		`SELECT COUNT(*) FROM library_furniture WHERE user_id = ?`,
		userID,
	).Scan(&libraryCount)

	w.Header().Set("Cache-Control", "no-store")
	a.renderTemplate(w, r, "hub", a.mergeData(r, map[string]any{
		"IsAdmin":              isAdmin == 1,
		"HubWorksProgress":     worksInProgress,
		"HubAnimeProgress":     animeInProgress,
		"HubBdProgress":        bdInProgress,
		"HubMangaPhysProgress": mangaPhysInProgress,
		"HubLibraryCount":      libraryCount,
	}))
}
