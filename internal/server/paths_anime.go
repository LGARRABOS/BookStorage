package server

// Route paths for the Anime module. Anime lives under the "/anime/" prefix,
// mirroring the Manga module. Anime API routes live at "/api/anime/...".
const (
	pathAnimeDashboard = "/anime/dashboard"
	pathAnimeCatalog   = "/anime/catalog"
	pathAnimeAddWork   = "/anime/add_work"

	// Prefix for routes carrying a path value (append the id).
	pathAnimeEditPrefix = "/anime/edit/"
)
