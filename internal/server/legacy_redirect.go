package server

import "net/http"

// RegisterLegacyRedirects maps the pre-Hub flat URLs to their new "/manga/"
// equivalents using 308 Permanent Redirects. A 308 preserves the HTTP method
// and body, so both bookmarked GET pages and in-flight POST forms keep working
// after the migration.
func (a *App) RegisterLegacyRedirects(mux *http.ServeMux) {
	redirect := func(to string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			target := to
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusPermanentRedirect)
		}
	}
	redirectID := func(prefix, param string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			target := prefix + r.PathValue(param)
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusPermanentRedirect)
		}
	}

	// Bookmarkable GET pages.
	mux.HandleFunc("/dashboard", redirect(pathMangaDashboard))
	mux.HandleFunc("/stats", redirect(pathMangaStats))
	mux.HandleFunc("/catalog", redirect(pathMangaCatalog))
	mux.HandleFunc("/add_work", redirect(pathMangaAddWork))
	mux.HandleFunc("/export", redirect(pathMangaExport))
	mux.HandleFunc("/reading-sites", redirect(pathMangaReadingSites))
	mux.HandleFunc("/users", redirect(pathMangaUsers))
	mux.HandleFunc("/edit/{id}", redirectID(pathMangaEditPrefix, "id"))
	mux.HandleFunc("/work/{id}", redirectID(pathMangaWorkPrefix, "id"))
	mux.HandleFunc("/users/{id}", redirectID(pathMangaUsersPrefix, "id"))

	// Pre-hub flat tools → new /tools/manga paths (index /tools is registered as a real page).
	mux.HandleFunc("/tools/csv-import", redirect(pathToolsMangaCSV))
	mux.HandleFunc("/tools/duplicates", redirect(pathToolsMangaDup))

	// Pre-tools-hub manga tools URLs → /tools/manga.
	mux.HandleFunc(pathMangaTools, redirect(pathToolsManga))
	mux.HandleFunc(pathMangaToolsCSV, redirect(pathToolsMangaCSV))
	mux.HandleFunc(pathMangaToolsDup, redirect(pathToolsMangaDup))

	// Form POST endpoints (308 preserves method + body).
	mux.HandleFunc("POST /import", redirect(pathMangaImport))
	mux.HandleFunc("POST /reading-sites/edit", redirect(pathMangaReadingSites+"/edit"))
	mux.HandleFunc("POST /reading-sites/delete", redirect(pathMangaReadingSites+"/delete"))
	mux.HandleFunc("POST /reading-sites/probe", redirect(pathMangaReadingSites+"/probe"))
	mux.HandleFunc("POST /reading-sites/probe-all", redirect(pathMangaReadingSites+"/probe-all"))
	mux.HandleFunc("POST /tools/duplicates/merge", redirect(pathToolsMangaDup+"/merge"))
	mux.HandleFunc("POST "+pathMangaToolsDup+"/merge", redirect(pathToolsMangaDup+"/merge"))
}
