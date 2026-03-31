package server

import (
	"context"
	"errors"
	"log"
	"os/exec"
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
}

type semVer struct {
	maj, min, pat int
	tag           string
}

var updateMu sync.Mutex
var updateRunning bool
var updateLast UpdateResult

func (a *App) triggerUpdate(ctx context.Context, mode updateMode) UpdateResult {
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
	updateMu.Unlock()

	defer func() {
		updateMu.Lock()
		updateRunning = false
		updateMu.Unlock()
	}()

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

	// Run bsctl update non-interactively. This assumes the server has permissions.
	// Try "bsctl" first (installed), fallback to repo script.
	log.Printf("[admin-update] requested mode=%s tag=%s", mode, tag)
	var cmd *exec.Cmd
	var execErr error
	for _, argv := range [][]string{{"bsctl", "update"}, {"./scripts/bsctl", "update"}} {
		cmd = exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Env = append(cmd.Environ(), "BSCTL_UPDATE_TAG="+tag)
		b, err := cmd.CombinedOutput()
		res.Output = strings.TrimSpace(string(b))
		if err == nil {
			execErr = nil
			break
		}
		execErr = err
		// If command not found, try next candidate; otherwise stop.
		var ee *exec.Error
		if errors.As(err, &ee) && errors.Is(ee.Err, exec.ErrNotFound) {
			continue
		}
		break
	}
	if execErr != nil {
		res.OK = false
		res.Message = "bsctl_failed"
	} else {
		res.OK = true
		res.Message = "started"
	}
	updateMu.Lock()
	updateLast = res
	updateMu.Unlock()
	return res
}

func (a *App) computeUpdateTag(mode updateMode) (tag string, output string, ok bool) {
	// Mirror scripts/bsctl.lib.sh semantics, but without shell dependencies:
	// - latest-major: newest tag matching vX.0.0
	// - latest: newest tag matching vX.Y.Z excluding vX.0.0
	cmd := exec.Command("git", "tag", "-l", "v*")
	b, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(b))
	if err != nil || out == "" {
		return "", out, false
	}
	lines := strings.Split(out, "\n")
	majorRe := regexp.MustCompile(`^v(\d+)\.0\.0$`)
	semverRe := regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

	var best *semVer
	for _, raw := range lines {
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
		return "", out, false
	}
	return best.tag, out, true
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
