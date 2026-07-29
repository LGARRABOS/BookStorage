package server

import "net/http"

// RegisterMangaPhysRoutes registers HTML pages for physical manga/webtoon (by tome).
func (a *App) RegisterMangaPhysRoutes(mux *http.ServeMux) {
	mux.HandleFunc(pathMangaPhysDashboard, a.RequireLogin(a.HandleMangaPhysDashboard))
	mux.HandleFunc(pathMangaPhysAddWork, a.RequireLogin(a.HandleMangaPhysAddWork))
	mux.HandleFunc(pathMangaPhysEditPrefix+"{id}", a.RequireLogin(a.HandleMangaPhysEditWork))
}
