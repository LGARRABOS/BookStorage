package server

import (
	"log"
	"strings"
	"sync"
	"time"

	"bookstorage/internal/catalog"
)

const (
	bdCoverEnrichBatchSize   = 100
	bdCoverEnrichMaxPasses   = 50
	bdCoverEnrichRateRetries = 3
)

// Overridable in tests to keep enrichment suites fast.
var (
	bdCoverEnrichPace           = 250 * time.Millisecond
	bdCoverEnrichInitialBackoff = 2 * time.Second
	bdCoverEnrichRateLimitSleep = time.Sleep
)

// resolveBdCoverURL looks up a cover image URL via Open Library (work key or title search).
// Overridable in tests.
var resolveBdCoverURL = resolveBdCoverURLDefault

type bdCoverResolve struct {
	URL     string
	IsAdult *bool
}

func resolveFromOpenLibraryBD(res *catalog.OpenLibraryBdResult) bdCoverResolve {
	if res == nil {
		return bdCoverResolve{}
	}
	return bdCoverResolve{URL: strings.TrimSpace(res.ImageURL), IsAdult: adultBoolPtr(res.IsAdult)}
}

// bdCoverSearchTitles returns title variants for Open Library cover lookup.
// BDGest-style titles often look like "Serie — Album"; try both full and parts.
func bdCoverSearchTitles(title string) []string {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	seen := map[string]struct{}{title: {}}
	out := []string{title}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, sep := range []string{" — ", " – ", " - "} {
		if i := strings.Index(title, sep); i > 0 {
			add(title[:i])
			add(title[i+len(sep):])
			break
		}
	}
	return out
}

func resolveBdCoverURLDefault(source, externalID, title string) (bdCoverResolve, error) {
	source = strings.ToLower(strings.TrimSpace(source))
	ext := strings.TrimSpace(externalID)
	// Only treat as Open Library id when source says so, or the key looks like /works/OL…W.
	if ext != "" && (source == "openlibrary" || source == "ol" || strings.HasPrefix(ext, "/works/")) {
		res, err := catalog.GetOpenLibraryBDByID(ext)
		if err != nil {
			if catalog.IsOpenLibraryRateLimit(err) {
				return bdCoverResolve{}, err
			}
			// Fall through to title search when the key is unknown.
		} else if res != nil && strings.TrimSpace(res.ImageURL) != "" {
			return resolveFromOpenLibraryBD(res), nil
		}
	}

	var lastErr error
	for _, q := range bdCoverSearchTitles(title) {
		results, err := catalog.SearchOpenLibraryBDCover(q, 5)
		if err != nil {
			lastErr = err
			if catalog.IsOpenLibraryRateLimit(err) {
				return bdCoverResolve{}, err
			}
			continue
		}
		for i := range results {
			if strings.TrimSpace(results[i].ImageURL) != "" {
				return resolveFromOpenLibraryBD(&results[i]), nil
			}
		}
	}
	if lastErr != nil {
		return bdCoverResolve{}, lastErr
	}
	return bdCoverResolve{}, nil
}

// bdCoverJobSnapshot is the admin-facing view of BD cover enrichment progress.
type bdCoverJobSnapshot struct {
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

type bdCoverJobController struct {
	mu sync.Mutex

	running  bool
	userID   int
	queue    []int
	workerOn bool

	pendingStart int
	processed    int
	filled       int
	skipped      int
	currentTitle string
	startedAt    time.Time
	rateLimited  bool
}

func (c *bdCoverJobController) enqueue(a *App, userID int) {
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
	if c.running && c.userID == userID {
		c.queue = append(c.queue, userID)
		return
	}
	c.queue = append(c.queue, userID)
	if !c.workerOn {
		c.workerOn = true
		go a.runBdCoverJobQueue()
	}
}

func (c *bdCoverJobController) begin(userID, pending int) {
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

func (c *bdCoverJobController) finish() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = false
	c.userID = 0
	c.currentTitle = ""
	c.rateLimited = false
}

func (c *bdCoverJobController) waitIdle(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		idle := !c.running && !c.workerOn && len(c.queue) == 0
		c.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (c *bdCoverJobController) setCurrent(title string) {
	c.mu.Lock()
	c.currentTitle = title
	c.mu.Unlock()
}

func (c *bdCoverJobController) setRateLimited(v bool) {
	c.mu.Lock()
	c.rateLimited = v
	c.mu.Unlock()
}

func (c *bdCoverJobController) markFilled() {
	c.mu.Lock()
	c.processed++
	c.filled++
	c.mu.Unlock()
}

func (c *bdCoverJobController) markSkipped() {
	c.mu.Lock()
	c.processed++
	c.skipped++
	c.mu.Unlock()
}

func (c *bdCoverJobController) snapshot(pendingNow, globalMissing int, username string) bdCoverJobSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := bdCoverJobSnapshot{
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
		eta := estimateAnimeCoverETA(pendingNow, c.processed, c.startedAt, bdCoverEnrichPace)
		if eta >= 0 {
			s.ETASeconds = &eta
		}
	} else if pendingNow > 0 && !c.running {
		eta := int(float64(pendingNow) * bdCoverEnrichPace.Seconds())
		s.ETASeconds = &eta
	}
	return s
}

// scheduleBdCoverEnrichment fills missing BD covers in the background after import.
func (a *App) scheduleBdCoverEnrichment(userID int) {
	if a == nil || a.DB == nil || userID <= 0 {
		return
	}
	a.bdCoverJobs.enqueue(a, userID)
}

func (a *App) runBdCoverJobQueue() {
	for {
		a.bdCoverJobs.mu.Lock()
		if len(a.bdCoverJobs.queue) == 0 {
			a.bdCoverJobs.workerOn = false
			a.bdCoverJobs.mu.Unlock()
			return
		}
		uid := a.bdCoverJobs.queue[0]
		a.bdCoverJobs.queue = a.bdCoverJobs.queue[1:]
		a.bdCoverJobs.mu.Unlock()

		pending := a.countBdMissingCovers(uid)
		a.bdCoverJobs.begin(uid, pending)
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("[bd-covers] queue panic user=%d: %v", uid, rec)
				}
			}()
			a.enrichBdCoversMissing(uid)
		}()
		a.bdCoverJobs.finish()
	}
}

func (a *App) countBdMissingCovers(userID int) int {
	var n int
	if userID > 0 {
		_ = a.DB.QueryRow(
			`SELECT COUNT(*) FROM bd_works
             WHERE user_id = ? AND (image_path IS NULL OR TRIM(image_path) = '')`,
			userID,
		).Scan(&n)
		return n
	}
	_ = a.DB.QueryRow(
		`SELECT COUNT(*) FROM bd_works
         WHERE image_path IS NULL OR TRIM(image_path) = ''`,
	).Scan(&n)
	return n
}

func (a *App) listUsersWithMissingBdCovers() []int {
	rows, err := a.DB.Query(
		`SELECT DISTINCT user_id FROM bd_works
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

func (a *App) bdCoverJobStatus() bdCoverJobSnapshot {
	pendingUser := 0
	username := ""
	a.bdCoverJobs.mu.Lock()
	running := a.bdCoverJobs.running
	uid := a.bdCoverJobs.userID
	a.bdCoverJobs.mu.Unlock()
	if running && uid > 0 {
		pendingUser = a.countBdMissingCovers(uid)
		_ = a.DB.QueryRow(`SELECT username FROM users WHERE id = ?`, uid).Scan(&username)
	}
	global := a.countBdMissingCovers(0)
	pendingNow := pendingUser
	if !running {
		pendingNow = global
	}
	return a.bdCoverJobs.snapshot(pendingNow, global, username)
}

func (a *App) enrichBdCoversMissing(userID int) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[bd-covers] panic user=%d: %v", userID, rec)
		}
	}()
	lastID := 0
	for pass := 0; pass < bdCoverEnrichMaxPasses; pass++ {
		list, err := a.loadBdMissingCovers(userID, lastID, bdCoverEnrichBatchSize)
		if err != nil || len(list) == 0 {
			return
		}
		for _, p := range list {
			lastID = p.id
			a.enrichOneBdCover(userID, p)
			time.Sleep(bdCoverEnrichPace)
		}
		if len(list) < bdCoverEnrichBatchSize {
			return
		}
	}
}

type bdCoverPending struct {
	id         int
	title      string
	source     string
	externalID string
}

func (a *App) loadBdMissingCovers(userID, afterID, limit int) ([]bdCoverPending, error) {
	rows, err := a.DB.Query(
		`SELECT id, title, COALESCE(source, ''), COALESCE(external_id, '')
         FROM bd_works
         WHERE user_id = ? AND id > ? AND (image_path IS NULL OR TRIM(image_path) = '')
         ORDER BY id
         LIMIT ?`,
		userID, afterID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var list []bdCoverPending
	for rows.Next() {
		var p bdCoverPending
		if err := rows.Scan(&p.id, &p.title, &p.source, &p.externalID); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (a *App) enrichOneBdCover(userID int, p bdCoverPending) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[bd-covers] panic user=%d id=%d title=%q: %v", userID, p.id, p.title, rec)
			a.bdCoverJobs.markSkipped()
		}
	}()
	a.bdCoverJobs.setCurrent(p.title)
	backoff := bdCoverEnrichInitialBackoff
	for attempt := 0; attempt < bdCoverEnrichRateRetries; attempt++ {
		got, err := resolveBdCoverURL(p.source, p.externalID, p.title)
		if catalog.IsOpenLibraryRateLimit(err) {
			a.bdCoverJobs.setRateLimited(true)
			log.Printf("[bd-covers] rate limit user=%d id=%d attempt=%d; sleeping %s", userID, p.id, attempt+1, backoff)
			bdCoverEnrichRateLimitSleep(backoff)
			if backoff < 15*time.Second {
				backoff *= 2
			}
			continue
		}
		a.bdCoverJobs.setRateLimited(false)
		if err != nil {
			log.Printf("[bd-covers] skip user=%d id=%d title=%q: %v", userID, p.id, p.title, err)
			a.bdCoverJobs.markSkipped()
			return
		}
		if got.URL == "" {
			a.bdCoverJobs.markSkipped()
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
			`UPDATE bd_works SET
             image_path = ?,
             is_adult = COALESCE(?, is_adult),
             updated_at = CURRENT_TIMESTAMP
             WHERE id = ? AND user_id = ? AND (image_path IS NULL OR TRIM(image_path) = '')`,
			got.URL, adultArg, p.id, userID,
		)
		if err != nil {
			log.Printf("[bd-covers] db update failed user=%d id=%d: %v", userID, p.id, err)
			a.bdCoverJobs.markSkipped()
			return
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			a.bdCoverJobs.markFilled()
		} else {
			a.bdCoverJobs.markSkipped()
		}
		return
	}
	a.bdCoverJobs.markSkipped()
}
