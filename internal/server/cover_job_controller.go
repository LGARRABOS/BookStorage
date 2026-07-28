package server

import (
	"sync"
	"time"
)

// coverJobCore holds progress state shared by BD and anime cover enrichment workers.
type coverJobCore struct {
	mu sync.Mutex

	running      bool
	userID       int
	pendingStart int
	processed    int
	filled       int
	skipped      int
	currentTitle string
	startedAt    time.Time
	rateLimited  bool
	workerOn     bool
}

func (c *coverJobCore) beginJob(userID, pending int) {
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

func (c *coverJobCore) finishJob() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = false
	c.userID = 0
	c.currentTitle = ""
	c.rateLimited = false
}

func (c *coverJobCore) waitIdle(queueEmpty func() bool, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		idle := !c.running && !c.workerOn && queueEmpty()
		c.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (c *coverJobCore) setCurrent(title string) {
	c.mu.Lock()
	c.currentTitle = title
	c.mu.Unlock()
}

func (c *coverJobCore) setRateLimited(v bool) {
	c.mu.Lock()
	c.rateLimited = v
	c.mu.Unlock()
}

func (c *coverJobCore) markFilled() {
	c.mu.Lock()
	c.processed++
	c.filled++
	c.mu.Unlock()
}

func (c *coverJobCore) markSkipped() {
	c.mu.Lock()
	c.processed++
	c.skipped++
	c.mu.Unlock()
}

func adultBoolPtr(v bool) *bool { return &v }

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
