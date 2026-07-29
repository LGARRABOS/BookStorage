package server

// Route paths for BookStorage.
//
// The Hub lives at the site root ("/") and is the landing page for logged-in
// users. The Manga/Webtoon module and all its HTML pages live under the
// "/manga/" prefix. API routes stay at the root ("/api/...") so existing API
// tokens and integrations keep working. Global, cross-module pages (profile,
// admin) also stay at the root.
const (
	pathHub         = "/"
	pathHubExplicit = "/hub"

	pathMangaDashboard    = "/manga/dashboard"
	pathMangaStats        = "/manga/stats"
	pathMangaCatalog      = "/manga/catalog"
	pathMangaAddWork      = "/manga/add_work"
	pathMangaReadingSites = "/manga/reading-sites"
	pathMangaUsers        = "/manga/users"
	pathMangaExport       = "/manga/export"
	pathMangaImport       = "/manga/import"

	// Legacy tools URLs (308 → /tools/manga/...). Kept for redirects.
	pathMangaTools    = "/manga/tools"
	pathMangaToolsCSV = "/manga/tools/csv-import"
	pathMangaToolsDup = "/manga/tools/duplicates"

	// Prefixes for routes carrying a path value (append the id).
	pathMangaEditPrefix  = "/manga/edit/"
	pathMangaWorkPrefix  = "/manga/work/"
	pathMangaUsersPrefix = "/manga/users/"
)
