package server

import (
	"log"
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

// animeCoverJobSnapshot is the admin-facing view of cover enrichment progress.
type animeCoverJobSnapshot struct {
	Running       bool   `json:"running"`
	UserID        int    `json:"user_id,omitempty"`
	Username      string `json:"username,omitempty"`
	PendingStart  int    `json:"pending_at_start"`
	PendingNow    int    `json:"pending_now"`
	Processed     int    `json:"processed"`
	Filled        int    `json:"filled"`
	Skipped       int    `json:"skipped"`
	CurrentTitle  string `json:"current_title,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	ETASeconds    *int   `json:"eta_seconds,omitempty"`
	RateLimited   bool   `json:"rate_limited"`
	QueuedUsers   []int  `json:"queued_user_ids,omitempty"`
	GlobalMissing int    `json:"global_missing"`
}

type animeCoverJobController struct {
	coverJobCore
	queue []int
}

func (c *animeCoverJobController) enqueue(a *App, userID int) {
	if a == nil || userID <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range c.queue {
		if id == userID {
			return
		}
	}
	// Already running for this user: schedule a follow-up pass after it finishes
	// so a concurrent re-import still gets cover enrichment.
	if c.running && c.userID == userID {
		c.queue = append(c.queue, userID)
		return
	}
	c.queue = append(c.queue, userID)
	if !c.workerOn {
		c.workerOn = true
		go a.runAnimeCoverJobQueue()
	}
}

func (c *animeCoverJobController) begin(userID, pending int) {
	c.beginJob(userID, pending)
}

func (c *animeCoverJobController) finish() {
	c.finishJob()
}

// waitIdle blocks until the cover job worker has drained its queue (or timeout).
func (c *animeCoverJobController) waitIdle(timeout time.Duration) {
	c.coverJobCore.waitIdle(func() bool { return len(c.queue) == 0 }, timeout)
}

func (c *animeCoverJobController) setCurrent(title string) {
	c.coverJobCore.setCurrent(title)
}

func (c *animeCoverJobController) setRateLimited(v bool) {
	c.coverJobCore.setRateLimited(v)
}

func (c *animeCoverJobController) markFilled() {
	c.coverJobCore.markFilled()
}

func (c *animeCoverJobController) markSkipped() {
	c.coverJobCore.markSkipped()
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
		got, err := resolveAnimeCoverURL(p.source, p.externalID, p.title)
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
		if err != nil || got.URL == "" {
			a.animeCoverJobs.markSkipped()
			return
		}
		adultArg := any(nil)
		if got.IsAdult != nil {
			if *got.IsAdult {
				adultArg = 1
			} else {
				adultArg = 0
			}
		}
		res, err := a.DB.Exec(
			`UPDATE anime_works SET
             image_path = ?,
             is_adult = COALESCE(?, is_adult),
             updated_at = CURRENT_TIMESTAMP
             WHERE id = ? AND user_id = ? AND (image_path IS NULL OR TRIM(image_path) = '')`,
			got.URL, adultArg, p.id, userID,
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
