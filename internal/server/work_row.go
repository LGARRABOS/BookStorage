package server

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type nullFlexTime struct {
	sql.NullString
}

func (n *nullFlexTime) Scan(src any) error {
	if src == nil {
		n.Valid, n.String = false, ""
		return nil
	}
	n.Valid = true
	switch v := src.(type) {
	case []byte:
		n.String = string(v)
	case string:
		n.String = v
	case time.Time:
		n.String = v.UTC().Format("2006-01-02 15:04:05")
	default:
		n.String = fmt.Sprint(v)
	}
	return nil
}

type workRow struct {
	ID                  int
	Title               string
	Chapter             int
	Link                sql.NullString
	Status              sql.NullString
	ImagePath           sql.NullString
	ReadingType         sql.NullString
	Rating              int
	Notes               sql.NullString
	UserID              int
	UpdatedAt           nullFlexTime
	IsAdult             sql.NullInt64
	ParentWorkID        sql.NullInt64
	SeriesSort          int
	NotifyNewChapters   int
	ReadingSiteID       sql.NullInt64
	StartedAt           nullFlexTime
	LastChapterAt       nullFlexTime
	FinishedAt          nullFlexTime
	LinkProbeStatus     sql.NullString
	LinkProbeAt         nullFlexTime
	LinkProbeHTTPStatus sql.NullInt64
	LinkProbeDetail     sql.NullString
}

// sqlWorkRowFull must match scanFullWorkRow field order.
const sqlWorkRowFull = `id, title, chapter, link, status, image_path, reading_type, COALESCE(rating, 0), notes, user_id, updated_at, COALESCE(is_adult, 0), parent_work_id, COALESCE(series_sort, 0), COALESCE(notify_new_chapters, 1), reading_site_id, started_at, last_chapter_at, finished_at, COALESCE(link_probe_status, 'unknown'), link_probe_at, link_probe_http_status, link_probe_detail`

func scanFullWorkRow(w *workRow, s interface{ Scan(dest ...any) error }) error {
	return s.Scan(
		&w.ID, &w.Title, &w.Chapter, &w.Link, &w.Status, &w.ImagePath, &w.ReadingType,
		&w.Rating, &w.Notes, &w.UserID, &w.UpdatedAt, &w.IsAdult, &w.ParentWorkID, &w.SeriesSort,
		&w.NotifyNewChapters, &w.ReadingSiteID, &w.StartedAt, &w.LastChapterAt, &w.FinishedAt,
		&w.LinkProbeStatus, &w.LinkProbeAt, &w.LinkProbeHTTPStatus, &w.LinkProbeDetail,
	)
}

// catalogSourcePageURL builds a public web page URL for a catalog row (AniList, MangaDex), or "".
func catalogSourcePageURL(source, externalID string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	ext := strings.TrimSpace(externalID)
	if ext == "" {
		return ""
	}
	switch source {
	case "anilist":
		return "https://anilist.co/manga/" + url.PathEscape(ext)
	case "mangadex":
		return "https://mangadex.org/title/" + url.PathEscape(ext)
	default:
		return ""
	}
}

func (a *App) catalogPageURLForUserWork(userID, workID int) string {
	var source sql.NullString
	var extID string
	err := a.DB.QueryRow(
		`SELECT c.source, COALESCE(c.external_id, '') FROM works w INNER JOIN catalog c ON c.id = w.catalog_id WHERE w.id = ? AND w.user_id = ?`,
		workID, userID,
	).Scan(&source, &extID)
	if err != nil || !source.Valid {
		return ""
	}
	return catalogSourcePageURL(source.String, extID)
}

// catalogAnilistCoverForUserWork returns the catalog cover URL when the work is linked to an AniList row (locked URL field in the edit form).
func (a *App) catalogAnilistCoverForUserWork(userID, workID int) (imageURL string, locked bool) {
	var source sql.NullString
	var catImg string
	err := a.DB.QueryRow(
		`SELECT c.source, COALESCE(c.image_url, '') FROM works w INNER JOIN catalog c ON c.id = w.catalog_id WHERE w.id = ? AND w.user_id = ?`,
		workID, userID,
	).Scan(&source, &catImg)
	if err != nil || !source.Valid {
		return "", false
	}
	if strings.ToLower(strings.TrimSpace(source.String)) != "anilist" {
		return "", false
	}
	return strings.TrimSpace(catImg), true
}

// effectiveLinkDotStatus matches dashboard display: link probe, or reading site status when link probe is still unknown.
func effectiveLinkDotStatus(w workRow, siteMap map[int]readingSite) string {
	if !w.Link.Valid || strings.TrimSpace(w.Link.String) == "" {
		return "none"
	}
	probe := "unknown"
	if w.LinkProbeStatus.Valid && strings.TrimSpace(w.LinkProbeStatus.String) != "" {
		probe = strings.TrimSpace(w.LinkProbeStatus.String)
	}
	if probe == "unknown" && w.ReadingSiteID.Valid {
		if rs, ok := siteMap[int(w.ReadingSiteID.Int64)]; ok {
			if rs.ProbeStatus == "down" || rs.ProbeStatus == "degraded" {
				return rs.ProbeStatus
			}
		}
	}
	return probe
}

func linkStatusIsDead(status string) bool {
	return status == "down" || status == "degraded"
}
