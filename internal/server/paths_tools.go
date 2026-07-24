package server

// Route paths for the cross-module Tools area under "/tools/".
// Manga-specific tools live under "/tools/manga/"; anime under "/tools/anime/".
// Legacy "/manga/tools" URLs 308-redirect here.
const (
	pathTools         = "/tools"
	pathToolsManga    = "/tools/manga"
	pathToolsMangaCSV = "/tools/manga/csv-import"
	pathToolsMangaDup = "/tools/manga/duplicates"
	pathToolsAnime    = "/tools/anime"

	pathAnimeExport = "/anime/export"
	pathAnimeImport = "/anime/import"
)
