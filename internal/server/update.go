package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type updateMode string

const (
	updateModeLatest      updateMode = "latest"
	updateModeLatestMajor updateMode = "latest-major"
)

type UpdateResult struct {
	OK        bool   `json:"ok"`
	Mode      string `json:"mode"`
	Tag       string `json:"tag,omitempty"`
	Message   string `json:"message,omitempty"`
	StartedAt int64  `json:"started_at_unix,omitempty"`
	Output    string `json:"output,omitempty"`
	Command   string `json:"command,omitempty"`
}

type semVer struct {
	maj, min, pat int
	tag           string
}

var updateMu sync.Mutex
var updateRunning bool
var updateLast UpdateResult

type updateRequest struct {
	Mode        string `json:"mode"`
	Tag         string `json:"tag"`
	RequestedAt int64  `json:"requested_at_unix"`
}

func parseSemVerTag(s string) (semVer, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return semVer{}, false
	}
	if !strings.HasPrefix(s, "v") {
		s = "v" + s
	}
	semverRe := regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return semVer{}, false
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	pat, _ := strconv.Atoi(m[3])
	return semVer{maj: maj, min: min, pat: pat, tag: s}, true
}

func (a *App) updateDir() string {
	// When set, admin updates are delegated to a root worker watching request.json in this directory.
	// Example: /var/lib/bookstorage/update
	d := strings.TrimSpace(os.Getenv("BOOKSTORAGE_UPDATE_DIR"))
	if d == "" {
		return ""
	}
	return d
}

func (a *App) updateRequestPath() string {
	d := a.updateDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "request.json")
}

func (a *App) updateStatusPath() string {
	d := a.updateDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "status.json")
}

func writeJSONAtomic(path string, v any) error {
	dir := filepath.Dir(path)
	tmp := filepath.Join(dir, ".tmp."+filepath.Base(path))
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSONFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (a *App) triggerUpdate(ctx context.Context, mode updateMode) UpdateResult {
	tag, out, ok := a.computeUpdateTag(mode)
	res := UpdateResult{
		OK:        ok,
		Mode:      string(mode),
		Tag:       tag,
		StartedAt: time.Now().Unix(),
		Output:    out,
	}
	if !ok {
		res.Message = "failed_to_resolve_tag"
		updateMu.Lock()
		updateLast = res
		updateMu.Unlock()
		return res
	}

	// If already on that version, behave like bsctl: no-op with a clear message.
	cur := strings.TrimSpace(a.Version)
	if cur != "" && cur != "dev" && tag != "" {
		curNoV := strings.TrimPrefix(cur, "v")
		targetNoV := strings.TrimPrefix(strings.TrimSpace(tag), "v")

		// Exact match: no-op.
		if curNoV == targetNoV {
			res.OK = true
			res.Message = "already_up_to_date"
			res.Output = ""
			updateMu.Lock()
			updateLast = res
			updateMu.Unlock()
			return res
		}

		// Prevent downgrades / pointless "major" updates (e.g. current v5.2.3 vs latest-major v5.0.0).
		if curV, ok1 := parseSemVerTag(curNoV); ok1 {
			if targetV, ok2 := parseSemVerTag(tag); ok2 {
				if compareVer(curV, targetV) >= 0 {
					res.OK = true
					res.Message = "already_up_to_date"
					res.Output = ""
					updateMu.Lock()
					updateLast = res
					updateMu.Unlock()
					return res
				}
			}
		}
	}

	updateMu.Lock()
	if updateRunning {
		r := updateLast
		r.OK = false
		r.Mode = string(mode)
		r.Message = "update_already_running"
		updateMu.Unlock()
		return r
	}
	updateRunning = true
	res.OK = true
	res.Message = "started"
	// We respond immediately to avoid the HTTP request being interrupted by service restarts.
	updateLast = res
	updateMu.Unlock()

	log.Printf("[admin-update] requested mode=%s tag=%s", mode, tag)

	// Version C: delegate to root worker via request/status files.
	if reqPath := a.updateRequestPath(); reqPath != "" {
		stPath := a.updateStatusPath()
		_ = writeJSONAtomic(stPath, UpdateStatus{Running: true, Last: res})
		req := updateRequest{Mode: string(mode), Tag: tag, RequestedAt: time.Now().Unix()}
		if err := writeJSONAtomic(reqPath, req); err == nil {
			// Mark as queued; the worker will update status.json.
			res.Message = "queued"
			updateMu.Lock()
			updateLast = res
			updateMu.Unlock()
			return res
		}
		// If writing fails (permissions/misconfig), fall back to in-process best-effort.
	}

	// IMPORTANT: do not use the request context, it will be cancelled when the handler returns.
	go a.runUpdateInBackground(mode, tag)
	return res
}

func (a *App) runUpdateInBackground(mode updateMode, tag string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	workDir := a.bestUpdateWorkDir()
	candidates := [][]string{
		// Prefer non-interactive sudo if available (common for production installs).
		{"sudo", "-n", "bsctl", "update"},
		{"sudo", "-n", "/usr/local/bin/bsctl", "update"},
		{"sudo", "-n", "/opt/bookstorage/scripts/bsctl", "update"},
		{"sudo", "-n", "./scripts/bsctl", "update"},
		{"bsctl", "update"},
		{"/usr/local/bin/bsctl", "update"},
		{"/opt/bookstorage/scripts/bsctl", "update"},
		{"./scripts/bsctl", "update"},
	}

	res := UpdateResult{
		OK:        false,
		Mode:      string(mode),
		Tag:       tag,
		StartedAt: time.Now().Unix(),
		Message:   "bsctl_failed",
	}

	var lastErr error
	var lastOut string
	var notFoundCount int
	for _, argv := range candidates {
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Env = append(cmd.Environ(), "BSCTL_UPDATE_TAG="+tag)
		if workDir != "" {
			cmd.Dir = workDir
		}

		b, err := cmd.CombinedOutput()
		out := strings.TrimSpace(string(b))
		lastOut = out
		lastErr = err
		res.Command = strings.Join(argv, " ")
		res.Output = out

		if err == nil {
			res.OK = true
			res.Message = "done"
			break
		}

		var ee *exec.Error
		if errors.As(err, &ee) && errors.Is(ee.Err, exec.ErrNotFound) {
			notFoundCount++
			continue
		}
		// Non-zero exit: stop at the first real failure (keep output).
		break
	}

	if !res.OK {
		if notFoundCount == len(candidates) {
			res.Message = "bsctl_not_found"
		} else {
			res.Message = "bsctl_failed"
		}
		if res.Output == "" && lastOut != "" {
			res.Output = lastOut
		}
		if res.Output == "" && lastErr != nil {
			res.Output = lastErr.Error()
		}
	}

	updateMu.Lock()
	updateLast = res
	updateRunning = false
	updateMu.Unlock()
}

type UpdateStatus struct {
	Running bool         `json:"running"`
	Last    UpdateResult `json:"last"`
}

func (a *App) updateStatus() UpdateStatus {
	// If Version C is configured, prefer on-disk status (survives restarts).
	if stPath := a.updateStatusPath(); stPath != "" {
		var st UpdateStatus
		if err := readJSONFile(stPath, &st); err == nil {
			return st
		}
	}
	updateMu.Lock()
	defer updateMu.Unlock()
	return UpdateStatus{Running: updateRunning, Last: updateLast}
}

func (a *App) bestUpdateWorkDir() string {
	// Prefer /opt/bookstorage in production installs, else current executable dir, else empty.
	if st, err := os.Stat("/opt/bookstorage"); err == nil && st.IsDir() {
		return "/opt/bookstorage"
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		d := filepath.Dir(exe)
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return ""
}

func (a *App) computeUpdateTag(mode updateMode) (tag string, output string, ok bool) {
	// Mirror scripts/bsctl.lib.sh semantics, but without shell dependencies:
	// - latest-major: newest tag matching vX.0.0
	// - latest: newest tag matching vX.Y.Z excluding vX.0.0
	tags, src, err := a.listAvailableTags()
	if err != nil || len(tags) == 0 {
		msg := src
		if err != nil {
			if msg != "" {
				msg += "\n"
			}
			msg += err.Error()
		}
		return "", strings.TrimSpace(msg), false
	}

	majorRe := regexp.MustCompile(`^v(\d+)\.0\.0$`)
	semverRe := regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

	var best *semVer
	for _, raw := range tags {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		if mode == updateModeLatestMajor {
			m := majorRe.FindStringSubmatch(t)
			if m == nil {
				continue
			}
			maj, _ := strconv.Atoi(m[1])
			v := semVer{maj: maj, min: 0, pat: 0, tag: t}
			if best == nil || compareVer(v, *best) > 0 {
				tmp := v
				best = &tmp
			}
			continue
		}

		m := semverRe.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		maj, _ := strconv.Atoi(m[1])
		min, _ := strconv.Atoi(m[2])
		pat, _ := strconv.Atoi(m[3])
		if min == 0 && pat == 0 {
			continue
		}
		v := semVer{maj: maj, min: min, pat: pat, tag: t}
		if best == nil || compareVer(v, *best) > 0 {
			tmp := v
			best = &tmp
		}
	}
	if best == nil || best.tag == "" {
		return "", src, false
	}
	return best.tag, src, true
}

func compareVer(a, b semVer) int {
	if a.maj != b.maj {
		if a.maj < b.maj {
			return -1
		}
		return 1
	}
	if a.min != b.min {
		if a.min < b.min {
			return -1
		}
		return 1
	}
	if a.pat != b.pat {
		if a.pat < b.pat {
			return -1
		}
		return 1
	}
	return 0
}

type githubTag struct {
	Name string `json:"name"`
}

func (a *App) listAvailableTags() (tags []string, source string, err error) {
	// 1) Try git tags locally from likely install locations.
	candidates := []string{"."}
	if exe, e := os.Executable(); e == nil && exe != "" {
		dir := filepath.Dir(exe)
		candidates = append(candidates, dir, filepath.Dir(dir))
	}
	// Common linux install dir for bsctl / repo.
	candidates = append(candidates, "/opt/bookstorage")

	seen := map[string]struct{}{}
	for _, d := range candidates {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}

		cmd := exec.Command("git", "-C", d, "tag", "-l", "v*")
		b, e := cmd.CombinedOutput()
		out := strings.TrimSpace(string(b))
		if e == nil && out != "" {
			lines := strings.Split(out, "\n")
			var res []string
			for _, ln := range lines {
				ln = strings.TrimSpace(ln)
				if ln != "" {
					res = append(res, ln)
				}
			}
			if len(res) > 0 {
				return res, "source=git dir=" + d, nil
			}
		}
	}

	// 2) Fallback to public GitHub tags (no auth). This keeps admin UI usable even without a git repo on disk.
	// NOTE: Repo is currently fixed to upstream; can be made configurable later.
	const repo = "LGARRABOS/BookStorage"
	url := "https://api.github.com/repos/" + repo + "/tags?per_page=100"

	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "bookstorage-admin-update")
	resp, e := client.Do(req)
	if e != nil {
		return nil, "source=github repo=" + repo, e
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "source=github repo=" + repo, errors.New("github_api_status_" + strconv.Itoa(resp.StatusCode))
	}
	var payload []githubTag
	if e := json.NewDecoder(resp.Body).Decode(&payload); e != nil {
		return nil, "source=github repo=" + repo, e
	}
	var res []string
	for _, t := range payload {
		name := strings.TrimSpace(t.Name)
		if name != "" {
			res = append(res, name)
		}
	}
	return res, "source=github repo=" + repo, nil
}
