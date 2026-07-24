package server

import "net/http"

// RegisterAnimeRoutes registers every HTML page route of the Anime module under
// the "/anime/" prefix. API routes (/api/anime/...) stay at the root and are
// registered in main.go, mirroring the Manga module.
func (a *App) RegisterAnimeRoutes(mux *http.ServeMux) {
	mux.HandleFunc(pathAnimeDashboard, a.RequireLogin(a.HandleAnimeDashboard))
	mux.HandleFunc(pathAnimeCatalog, a.RequireLogin(a.HandleAnimeCatalog))
	mux.HandleFunc(pathAnimeAddWork, a.RequireLogin(a.HandleAnimeAddWork))
	mux.HandleFunc(pathAnimeEditPrefix+"{id}", a.RequireLogin(a.HandleAnimeEditWork))
}
