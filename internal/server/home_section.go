package server

import "strings"

// Home section keys stored in users.home_section.
const (
	homeSectionHub       = "hub"
	homeSectionManga     = "manga"
	homeSectionMangaPhys = "manga_phys"
	homeSectionAnime     = "anime"
	homeSectionBd        = "bd"
	homeSectionLibrary   = "library"
)

var homeSectionChoices = []string{
	homeSectionHub,
	homeSectionManga,
	homeSectionMangaPhys,
	homeSectionAnime,
	homeSectionBd,
	homeSectionLibrary,
}

func normalizeHomeSection(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	for _, v := range homeSectionChoices {
		if s == v {
			return v
		}
	}
	return homeSectionHub
}

func pathForHomeSection(section string) string {
	switch normalizeHomeSection(section) {
	case homeSectionManga:
		return pathMangaDashboard
	case homeSectionMangaPhys:
		return pathMangaPhysDashboard
	case homeSectionAnime:
		return pathAnimeDashboard
	case homeSectionBd:
		return pathBdDashboard
	case homeSectionLibrary:
		return pathLibraryHome
	default:
		return pathHub
	}
}

func (a *App) userHomeSection(userID int) string {
	if a == nil || a.DB == nil || userID <= 0 {
		return homeSectionHub
	}
	var raw string
	err := a.DB.QueryRow(`SELECT COALESCE(home_section, '') FROM users WHERE id = ?`, userID).Scan(&raw)
	if err != nil {
		return homeSectionHub
	}
	return normalizeHomeSection(raw)
}

func (a *App) userHomePath(userID int) string {
	return pathForHomeSection(a.userHomeSection(userID))
}
