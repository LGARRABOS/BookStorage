package server

import (
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"bookstorage/internal/catalog"
)

const (
	animeCoverEnrichBatchSize   = 100
	animeCoverEnrichMaxPasses   = 50
	animeCoverEnrichRateRetries = 8
)

// Overridable in tests to keep enrichment suites fast.
var (
	animeCoverEnrichPace           = 650 * time.Millisecond
	animeCoverEnrichInitialBackoff = 30 * time.Second
	animeCoverEnrichRateLimitSleep = time.Sleep
)

// resolveAnimeCoverURL looks up a cover image URL via AniList (MAL id, AniList id, or title search).
// Overridable in tests. A rate-limit error should be retried by the caller; other empty results are final for this pass.
var resolveAnimeCoverURL = resolveAnimeCoverURLDefault

func resolveAnimeCoverURLDefault(source, externalID, title string) (string, error) {
	source = strings.ToLower(strings.TrimSpace(source))
	ext := strings.TrimSpace(externalID)
	if id, err := strconv.Atoi(ext); err == nil && id > 0 {
		switch source {
		case "anilist":
			res, err := catalog.GetAnilistAnimeByID(id)
			if err != nil {
				return "", err
			}
			if res != nil {
				return strings.TrimSpace(res.ImageURL), nil
			}
		case "mal", "myanimelist":
			res, err := catalog.GetAnilistAnimeByMALID(id)
			if err != nil {
				return "", err
			}
			if res != nil {
				return strings.TrimSpace(res.ImageURL), nil
			}
		default:
			if res, err := catalog.GetAnilistAnimeByID(id); err != nil {
				return "", err
			} else if res != nil {
				return strings.TrimSpace(res.ImageURL), nil
			}
			if res, err := catalog.GetAnilistAnimeByMALID(id); err != nil {
				return "", err
			} else if res != nil {
				return strings.TrimSpace(res.ImageURL), nil
			}
		}
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return "", nil
	}
	results, err := catalog.SearchAnilistAnime(title, 1)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", nil
	}
	return strings.TrimSpace(results[0].ImageURL), nil
}

// animeCoverJobSnapshot is the admin-facing view of cover enrichment progress.
type animeCoverJobSnapshot struct {
	Running      bool     `json:"running"`
	UserID       int      `json:"user_id,omitempty"`
	Username     string   `json:"username,omitempty"`
	PendingStart int      `json:"pending_at_start"`
	PendingNow   int      `json:"pending_now"`
	Processed    int      `json:"processed"`
	Filled       int      `json:"filled"`
	Skipped      int      `json:"skipped"`
	CurrentTitle string   `json:"current_title,omitempty"`
	StartedAt    string   `json:"started_at,omitempty"`
	ETASeconds   *int     `json:"eta_seconds,omitempty"`
	RateLimited  bool     `json:"rate_limited"`
	QueuedUsers  []int    `json:"queued_user_ids,omitempty"`
	GlobalMissing int     `json:"global_missing"`
}

type animeCoverJobController struct {
	mu sync.Mutex

	running   bool
	userID    int
	queue     []int
	workerOn  bool

	pendingStart int
	processed    int
	filled       int
	skipped      int
	currentTitle string
	startedAt    time.Time
	rateLimited  bool
}

func (c *animeCoverJobController) enqueue(a *App, userID int) {
	if a == nil || userID <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running && c.userID == userID {
		return
	}
	for _, id := range c.queue {
		if id == userID {
			return
		}
	}
	c.queue = append(c.queue, userID)
	if !c.workerOn {
		c.workerOn = true
		go a.runAnimeCoverJobQueue()
	}
}

func (c *animeCoverJobController) begin(userID, pending int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = true
	c.userID = userID
	c.pendingStart = pending
	c.processed = 0
	c.filled = 0
	c.skipped = 0
	c.currentTitle = ""
	c.startedAt = time.Now().UTC()
	c.rateLimited = false
}

func (c *animeCoverJobController) finish() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = false
	c.userID = 0
	c.currentTitle = ""
	c.rateLimited = false
}

func (c *animeCoverJobController) setCurrent(title string) {
	c.mu.Lock()
	c.currentTitle = title
	c.mu.Unlock()
}

func (c *animeCoverJobController) setRateLimited(v bool) {
	c.mu.Lock()
	c.rateLimited = v
	c.mu.Unlock()
}

func (c *animeCoverJobController) markFilled() {
	c.mu.Lock()
	c.processed++
	c.filled++
	c.mu.Unlock()
}

func (c *animeCoverJobController) markSkipped() {
	c.mu.Lock()
	c.processed++
	c.skipped++
	c.mu.Unlock()
}

func (c *animeCoverJobController) snapshot(pendingNow, globalMissing int, username string) animeCoverJobSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := animeCoverJobSnapshot{
		Running:       c.running,
		UserID:        c.userID,
		Username:      username,
		PendingStart:  c.pendingStart,
		PendingNow:    pendingNow,
		Processed:     c.processed,
		Filled:        c.filled,
		Skipped:       c.skipped,
		CurrentTitle:  c.currentTitle,
		RateLimited:   c.rateLimited,
		GlobalMissing: globalMissing,
		QueuedUsers:   append([]int(nil), c.queue...),
	}
	if !c.startedAt.IsZero() && c.running {
		s.StartedAt = c.startedAt.UTC().Format(time.RFC3339)
		eta := estimateAnimeCoverETA(pendingNow, c.processed, c.startedAt, animeCoverEnrichPace)
		if eta >= 0 {
			s.ETASeconds = &eta
		}
	} else if pendingNow > 0 && !c.running {
		eta := int(float64(pendingNow) * animeCoverEnrichPace.Seconds())
		s.ETASeconds = &eta
	}
	return s
}

func estimateAnimeCoverETA(pending, processed int, startedAt time.Time, pace time.Duration) int {
	if pending <= 0 {
		return 0
	}
	if processed >= 3 && !startedAt.IsZero() {
		elapsed := time.Since(startedAt).Seconds()
		if elapsed > 0 {
			perItem := elapsed / float64(processed)
			if perItem > 0 {
				return int(float64(pending) * perItem)
			}
		}
	}
	if pace <= 0 {
		return -1
	}
	return int(float64(pending) * pace.Seconds())
}

// scheduleAnimeCoverEnrichment fills missing anime covers in the background after import.
func (a *App) scheduleAnimeCoverEnrichment(userID int) {
	if a == nil || a.DB == nil || userID <= 0 {
		return
	}
	a.animeCoverJobs.enqueue(a, userID)
}

func (a *App) runAnimeCoverJobQueue() {
	for {
		a.animeCoverJobs.mu.Lock()
		if len(a.animeCoverJobs.queue) == 0 {
			a.animeCoverJobs.workerOn = false
			a.animeCoverJobs.mu.Unlock()
			return
		}
		uid := a.animeCoverJobs.queue[0]
		a.animeCoverJobs.queue = a.animeCoverJobs.queue[1:]
		a.animeCoverJobs.mu.Unlock()

		pending := a.countAnimeMissingCovers(uid)
		a.animeCoverJobs.begin(uid, pending)
		a.enrichAnimeCoversMissing(uid)
		a.animeCoverJobs.finish()
	}
}

func (a *App) countAnimeMissingCovers(userID int) int {
	var n int
	if userID > 0 {
		_ = a.DB.QueryRow(
			`SELECT COUNT(*) FROM anime_works
             WHERE user_id = ? AND (image_path IS NULL OR TRIM(image_path) = '')`,
			userID,
		).Scan(&n)
		return n
	}
	_ = a.DB.QueryRow(
		`SELECT COUNT(*) FROM anime_works
         WHERE image_path IS NULL OR TRIM(image_path) = ''`,
	).Scan(&n)
	return n
}

func (a *App) listUsersWithMissingAnimeCovers() []int {
	rows, err := a.DB.Query(
		`SELECT DISTINCT user_id FROM anime_works
         WHERE image_path IS NULL OR TRIM(image_path) = ''
         ORDER BY user_id`,
	)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return ids
		}
		ids = append(ids, id)
	}
	return ids
}

func (a *App) animeCoverJobStatus() animeCoverJobSnapshot {
	pendingUser := 0
	username := ""
	a.animeCoverJobs.mu.Lock()
	running := a.animeCoverJobs.running
	uid := a.animeCoverJobs.userID
	a.animeCoverJobs.mu.Unlock()
	if running && uid > 0 {
		pendingUser = a.countAnimeMissingCovers(uid)
		_ = a.DB.QueryRow(`SELECT username FROM users WHERE id = ?`, uid).Scan(&username)
	}
	global := a.countAnimeMissingCovers(0)
	pendingNow := pendingUser
	if !running {
		pendingNow = global
	}
	return a.animeCoverJobs.snapshot(pendingNow, global, username)
}

func (a *App) enrichAnimeCoversMissing(userID int) {
	lastID := 0
	for pass := 0; pass < animeCoverEnrichMaxPasses; pass++ {
		list, err := a.loadAnimeMissingCovers(userID, lastID, animeCoverEnrichBatchSize)
		if err != nil || len(list) == 0 {
			return
		}
		for _, p := range list {
			lastID = p.id
			a.enrichOneAnimeCover(userID, p)
			time.Sleep(animeCoverEnrichPace)
		}
		if len(list) < animeCoverEnrichBatchSize {
			return
		}
	}
}

type animeCoverPending struct {
	id         int
	title      string
	source     string
	externalID string
}

func (a *App) loadAnimeMissingCovers(userID, afterID, limit int) ([]animeCoverPending, error) {
	rows, err := a.DB.Query(
		`SELECT id, title, COALESCE(source, ''), COALESCE(external_id, '')
         FROM anime_works
         WHERE user_id = ? AND id > ? AND (image_path IS NULL OR TRIM(image_path) = '')
         ORDER BY id
         LIMIT ?`,
		userID, afterID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var list []animeCoverPending
	for rows.Next() {
		var p animeCoverPending
		if err := rows.Scan(&p.id, &p.title, &p.source, &p.externalID); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (a *App) enrichOneAnimeCover(userID int, p animeCoverPending) {
	a.animeCoverJobs.setCurrent(p.title)
	backoff := animeCoverEnrichInitialBackoff
	for attempt := 0; attempt < animeCoverEnrichRateRetries; attempt++ {
		cover, err := resolveAnimeCoverURL(p.source, p.externalID, p.title)
		if catalog.IsAnilistRateLimit(err) {
			a.animeCoverJobs.setRateLimited(true)
			log.Printf("[anime-covers] rate limit user=%d id=%d attempt=%d; sleeping %s", userID, p.id, attempt+1, backoff)
			animeCoverEnrichRateLimitSleep(backoff)
			if backoff < 2*time.Minute {
				backoff *= 2
			}
			continue
		}
		a.animeCoverJobs.setRateLimited(false)
		if err != nil || cover == "" {
			a.animeCoverJobs.markSkipped()
			return
		}
		res, err := a.DB.Exec(
			`UPDATE anime_works SET image_path = ?, updated_at = CURRENT_TIMESTAMP
             WHERE id = ? AND user_id = ? AND (image_path IS NULL OR TRIM(image_path) = '')`,
			cover, p.id, userID,
		)
		if err != nil {
			a.animeCoverJobs.markSkipped()
			return
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			a.animeCoverJobs.markFilled()
		} else {
			a.animeCoverJobs.markSkipped()
		}
		return
	}
	a.animeCoverJobs.markSkipped()
}
