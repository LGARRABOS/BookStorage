package server

import "net/http"

// RegisterBdRoutes registers HTML pages of the BD module under "/bd/".
func (a *App) RegisterBdRoutes(mux *http.ServeMux) {
	mux.HandleFunc(pathBdDashboard, a.RequireLogin(a.HandleBdDashboard))
	mux.HandleFunc(pathBdCatalog, a.RequireLogin(a.HandleBdCatalog))
	mux.HandleFunc(pathBdAddWork, a.RequireLogin(a.HandleBdAddWork))
	mux.HandleFunc(pathBdEditPrefix+"{id}", a.RequireLogin(a.HandleBdEditWork))
}
