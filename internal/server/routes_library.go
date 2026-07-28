package server

import "net/http"

// RegisterLibraryRoutes registers HTML pages of the physical library module.
func (a *App) RegisterLibraryRoutes(mux *http.ServeMux) {
	mux.HandleFunc(pathLibraryHome, a.RequireLogin(a.HandleLibraryHome))
	mux.HandleFunc(pathLibraryUnassigned, a.RequireLogin(a.HandleLibraryUnassigned))
	mux.HandleFunc(pathLibraryFurnitureNew, a.RequireLogin(a.HandleLibraryFurnitureNew))
	mux.HandleFunc(pathLibraryFurniturePrefix+"{id}", a.RequireLogin(a.HandleLibraryFurnitureView))
	mux.HandleFunc(pathLibraryFurniturePrefix+"{id}/edit", a.RequireLogin(a.HandleLibraryFurnitureEdit))
	mux.HandleFunc(pathLibraryFurniturePrefix+"{id}/delete", a.RequireLogin(a.HandleLibraryFurnitureDelete))
}
