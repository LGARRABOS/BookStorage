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

// resolveBdCoverURL looks up a cover via BnF / Google Books / Open Library.
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

func firstCoverURL(fns ...func() (string, error)) (string, error) {
	var lastErr error
	for _, fn := range fns {
		u, err := fn()
		if err != nil {
			if catalog.IsOpenLibraryRateLimit(err) {
				return "", err
			}
			lastErr = err
			continue
		}
		if strings.TrimSpace(u) != "" {
			return strings.TrimSpace(u), nil
		}
	}
	return "", lastErr
}

func resolveBdCoverURLDefault(source, externalID, title, isbn string) (bdCoverResolve, error) {
	source = strings.ToLower(strings.TrimSpace(source))
	ext := strings.TrimSpace(externalID)
	isbn = strings.TrimSpace(isbn)

	if isbn != "" {
		u, err := firstCoverURL(
			func() (string, error) { return catalog.LookupBnFCoverByISBN(isbn) },
			func() (string, error) { return catalog.LookupGoogleBooksCoverByISBN(isbn) },
			func() (string, error) { return catalog.OpenLibraryCoverByISBN(isbn) },
		)
		if err != nil {
			return bdCoverResolve{}, err
		}
		if u != "" {
			return bdCoverResolve{URL: u}, nil
		}
	}

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
		u, err := catalog.LookupGoogleBooksCoverByTitle(q)
		if err != nil {
			lastErr = err
			if catalog.IsOpenLibraryRateLimit(err) {
				return bdCoverResolve{}, err
			}
		} else if strings.TrimSpace(u) != "" {
			return bdCoverResolve{URL: strings.TrimSpace(u)}, nil
		}

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
	Mode          string `json:"mode,omitempty"` // "missing" | "replace"
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

type bdCoverQueueItem struct {
	userID  int
	replace bool
}

type bdCoverJobController struct {
	mu sync.Mutex

	running  bool
	userID   int
	replace  bool
	queue    []bdCoverQueueItem
	workerOn bool

	pendingStart int
	processed    int
	filled       int
	skipped      int
	currentTitle string
	startedAt    time.Time
	rateLimited  bool
}

func (c *bdCoverJobController) enqueue(a *App, userID int, replace bool) {
	if a == nil || userID <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, item := range c.queue {
		if item.userID == userID {
			if replace {
				c.queue[i].replace = true
			}
			return
		}
	}
	if c.running && c.userID == userID && c.replace == replace {
		// Same mode already running: queue another pass so late imports are covered.
		c.queue = append(c.queue, bdCoverQueueItem{userID: userID, replace: replace})
		return
	}
	c.queue = append(c.queue, bdCoverQueueItem{userID: userID, replace: replace})
	if !c.workerOn {
		c.workerOn = true
		go a.runBdCoverJobQueue()
	}
}

func (c *bdCoverJobController) begin(userID, pending int, replace bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = true
	c.userID = userID
	c.replace = replace
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
	c.replace = false
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
	mode := ""
	if c.running {
		if c.replace {
			mode = "replace"
		} else {
			mode = "missing"
		}
	}
	queued := make([]int, 0, len(c.queue))
	for _, item := range c.queue {
		queued = append(queued, item.userID)
	}
	s := bdCoverJobSnapshot{
		Running:       c.running,
		UserID:        c.userID,
		Username:      username,
		Mode:          mode,
		PendingStart:  c.pendingStart,
		PendingNow:    pendingNow,
		Processed:     c.processed,
		Filled:        c.filled,
		Skipped:       c.skipped,
		CurrentTitle:  c.currentTitle,
		RateLimited:   c.rateLimited,
		GlobalMissing: globalMissing,
		QueuedUsers:   queued,
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
	a.bdCoverJobs.enqueue(a, userID, false)
}

// scheduleBdCoverReplace refreshes covers for all BD works of a user (overwrites existing images).
func (a *App) scheduleBdCoverReplace(userID int) {
	if a == nil || a.DB == nil || userID <= 0 {
		return
	}
	a.bdCoverJobs.enqueue(a, userID, true)
}

func (a *App) runBdCoverJobQueue() {
	for {
		a.bdCoverJobs.mu.Lock()
		if len(a.bdCoverJobs.queue) == 0 {
			a.bdCoverJobs.workerOn = false
			a.bdCoverJobs.mu.Unlock()
			return
		}
		item := a.bdCoverJobs.queue[0]
		a.bdCoverJobs.queue = a.bdCoverJobs.queue[1:]
		a.bdCoverJobs.mu.Unlock()

		pending := a.countBdCoverTargets(item.userID, item.replace)
		a.bdCoverJobs.begin(item.userID, pending, item.replace)
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("[bd-covers] queue panic user=%d replace=%v: %v", item.userID, item.replace, rec)
				}
			}()
			a.enrichBdCovers(item.userID, item.replace)
		}()
		a.bdCoverJobs.finish()
	}
}

func (a *App) countBdMissingCovers(userID int) int {
	return a.countBdCoverTargets(userID, false)
}

func (a *App) countBdCoverTargets(userID int, replace bool) int {
	var n int
	missingClause := `AND (image_path IS NULL OR TRIM(image_path) = '')`
	if replace {
		missingClause = ""
	}
	if userID > 0 {
		_ = a.DB.QueryRow(
			`SELECT COUNT(*) FROM bd_works WHERE user_id = ? `+missingClause,
			userID,
		).Scan(&n)
		return n
	}
	_ = a.DB.QueryRow(`SELECT COUNT(*) FROM bd_works WHERE 1=1 ` + missingClause).Scan(&n)
	return n
}

func (a *App) listUsersWithMissingBdCovers() []int {
	return a.listUsersWithBdCoverTargets(false)
}

func (a *App) listUsersWithBdWorks() []int {
	return a.listUsersWithBdCoverTargets(true)
}

func (a *App) listUsersWithBdCoverTargets(replace bool) []int {
	q := `SELECT DISTINCT user_id FROM bd_works
         WHERE image_path IS NULL OR TRIM(image_path) = ''
         ORDER BY user_id`
	if replace {
		q = `SELECT DISTINCT user_id FROM bd_works ORDER BY user_id`
	}
	rows, err := a.DB.Query(q)
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
	replace := a.bdCoverJobs.replace
	a.bdCoverJobs.mu.Unlock()
	if running && uid > 0 {
		pendingUser = a.countBdCoverTargets(uid, replace)
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
	a.enrichBdCovers(userID, false)
}

func (a *App) enrichBdCovers(userID int, replace bool) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[bd-covers] panic user=%d replace=%v: %v", userID, replace, rec)
		}
	}()
	lastID := 0
	for pass := 0; pass < bdCoverEnrichMaxPasses; pass++ {
		list, err := a.loadBdCoverTargets(userID, lastID, bdCoverEnrichBatchSize, replace)
		if err != nil || len(list) == 0 {
			return
		}
		for _, p := range list {
			lastID = p.id
			a.enrichOneBdCover(userID, p, replace)
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
	isbn       string
}

func (a *App) loadBdCoverTargets(userID, afterID, limit int, replace bool) ([]bdCoverPending, error) {
	missingClause := `AND (image_path IS NULL OR TRIM(image_path) = '')`
	if replace {
		missingClause = ""
	}
	rows, err := a.DB.Query(
		`SELECT id, title, COALESCE(source, ''), COALESCE(external_id, ''), COALESCE(isbn, '')
         FROM bd_works
         WHERE user_id = ? AND id > ? `+missingClause+`
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
		if err := rows.Scan(&p.id, &p.title, &p.source, &p.externalID, &p.isbn); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (a *App) enrichOneBdCover(userID int, p bdCoverPending, replace bool) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[bd-covers] panic user=%d id=%d title=%q: %v", userID, p.id, p.title, rec)
			a.bdCoverJobs.markSkipped()
		}
	}()
	a.bdCoverJobs.setCurrent(p.title)
	backoff := bdCoverEnrichInitialBackoff
	for attempt := 0; attempt < bdCoverEnrichRateRetries; attempt++ {
		got, err := resolveBdCoverURL(p.source, p.externalID, p.title, p.isbn)
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
		q := `UPDATE bd_works SET
             image_path = ?,
             is_adult = COALESCE(?, is_adult),
             updated_at = CURRENT_TIMESTAMP
             WHERE id = ? AND user_id = ?`
		args := []any{got.URL, adultArg, p.id, userID}
		if !replace {
			q += ` AND (image_path IS NULL OR TRIM(image_path) = '')`
		}
		res, err := a.DB.Exec(q, args...)
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
