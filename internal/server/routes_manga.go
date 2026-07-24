package server

import "net/http"

// RegisterMangaRoutes registers every HTML page route of the Manga/Webtoon
// module under the "/manga/" prefix. API routes (/api/...) stay at the root and
// are registered in main.go so API tokens and integrations are unaffected.
func (a *App) RegisterMangaRoutes(mux *http.ServeMux) {
	mux.HandleFunc(pathMangaDashboard, a.RequireLogin(a.HandleDashboard))
	mux.HandleFunc(pathMangaStats, a.RequireLogin(a.HandleStats))
	mux.HandleFunc(pathMangaCatalog, a.RequireLogin(a.HandleCatalog))
	mux.HandleFunc(pathMangaAddWork, a.RequireLogin(a.HandleAddWork))
	mux.HandleFunc(pathMangaEditPrefix+"{id}", a.RequireLogin(a.HandleEditWork))
	mux.HandleFunc(pathMangaWorkPrefix+"{id}", a.RequireLogin(a.HandleWorkDetail))

	mux.HandleFunc(pathMangaReadingSites, a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleReadingSites)))
	mux.HandleFunc("POST "+pathMangaReadingSites+"/edit", a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleReadingSiteEdit)))
	mux.HandleFunc("POST "+pathMangaReadingSites+"/delete", a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleReadingSiteDelete)))
	mux.HandleFunc("POST "+pathMangaReadingSites+"/probe", a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleReadingSiteProbe)))
	mux.HandleFunc("POST "+pathMangaReadingSites+"/probe-all", a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleReadingSiteProbeAll)))

	mux.HandleFunc(pathMangaUsers, a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleUsers)))
	mux.HandleFunc(pathMangaUsersPrefix+"{id}", a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleUserDetail)))
	mux.HandleFunc("POST "+pathMangaUsersPrefix+"{user_id}/import/{work_id}", a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleImportWork)))

	mux.HandleFunc(pathMangaExport, a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleExport)))
	mux.HandleFunc("POST "+pathMangaImport, a.RequireLogin(a.MobileRedirectToMangaDashboard(a.HandleImport)))
}
