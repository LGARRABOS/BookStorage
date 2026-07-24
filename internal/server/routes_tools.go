package server

import "net/http"

// RegisterToolsRoutes registers the hub-level Tools pages under "/tools/"
// and anime import/export endpoints. Manga CSV-import/duplicates/export keep
// their handlers but are served from "/tools/manga/...".
func (a *App) RegisterToolsRoutes(mux *http.ServeMux) {
	mux.HandleFunc(pathTools, a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleToolsIndex)))
	mux.HandleFunc(pathToolsManga, a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleToolsManga)))
	mux.HandleFunc(pathToolsMangaCSV, a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleToolsCSVImport)))
	mux.HandleFunc(pathToolsMangaDup, a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleDuplicates)))
	mux.HandleFunc("POST "+pathToolsMangaDup+"/merge", a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleMergeDuplicate)))

	mux.HandleFunc(pathToolsAnime, a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleToolsAnime)))
	mux.HandleFunc(pathAnimeExport, a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleAnimeExport)))
	mux.HandleFunc("POST "+pathAnimeImport, a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleAnimeImport)))
}
