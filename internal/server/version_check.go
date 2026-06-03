package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const versionCheckTimeout = 8 * time.Second

var githubReleasesLatestURL = "https://api.github.com/repos/LGARRABOS/BookStorage/releases/latest"

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

var versionCheckHTTPClient = &http.Client{Timeout: versionCheckTimeout}

// parseSemver extracts major.minor.patch from a version or tag (optional "v" prefix).
func parseSemver(raw string) (major, minor, patch int, ok bool) {
	v := strings.TrimSpace(raw)
	v = strings.TrimPrefix(strings.ToLower(v), "v")
	if v == "" || v == "dev" {
		return 0, 0, 0, false
	}
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return 0, 0, 0, false
	}
	parsePart := func(s string) (int, bool) {
		if s == "" {
			return 0, false
		}
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return 0, false
		}
		return n, true
	}
	major, ok = parsePart(parts[0])
	if !ok {
		return 0, 0, 0, false
	}
	if len(parts) > 1 {
		minor, ok = parsePart(parts[1])
		if !ok {
			return 0, 0, 0, false
		}
	}
	if len(parts) > 2 {
		patch, ok = parsePart(parts[2])
		if !ok {
			return 0, 0, 0, false
		}
	}
	return major, minor, patch, true
}

func semverCompare(a, b string) int {
	am, ai, ap, aok := parseSemver(a)
	bm, bi, bp, bok := parseSemver(b)
	if !aok && !bok {
		return 0
	}
	if !aok {
		return -1
	}
	if !bok {
		return 1
	}
	if am != bm {
		return cmpInt(am, bm)
	}
	if ai != bi {
		return cmpInt(ai, bi)
	}
	if ap != bp {
		return cmpInt(ap, bp)
	}
	return 0
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func fetchLatestRelease(client *http.Client) (tagName, htmlURL string, ok bool) {
	if client == nil {
		client = versionCheckHTTPClient
	}
	req, err := http.NewRequest(http.MethodGet, githubReleasesLatestURL, nil)
	if err != nil {
		return "", "", false
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "BookStorage")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", false
	}
	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", "", false
	}
	tagName = strings.TrimSpace(rel.TagName)
	if tagName == "" {
		return "", "", false
	}
	return tagName, strings.TrimSpace(rel.HTMLURL), true
}

// checkUpdateAvailable compares the running version with the latest GitHub release.
// Returns available=true when a newer release exists.
func (a *App) checkUpdateAvailable() (available bool, latestVersion, releaseURL string) {
	current := strings.TrimSpace(a.Version)
	if current == "" || strings.EqualFold(current, "dev") {
		return false, "", ""
	}
	if _, _, _, ok := parseSemver(current); !ok {
		return false, "", ""
	}

	tag, url, ok := fetchLatestRelease(versionCheckHTTPClient)
	if !ok {
		return false, "", ""
	}
	if semverCompare(current, tag) >= 0 {
		return false, "", ""
	}
	return true, tag, url
}

// adminUpdateData returns template fields for the admin update banner.
func (a *App) adminUpdateData() map[string]any {
	available, latest, url := a.checkUpdateAvailable()
	data := map[string]any{
		"UpdateAvailable": available,
	}
	if available {
		data["LatestVersion"] = latest
		if url != "" {
			data["ReleaseURL"] = url
		}
		log.Printf("[admin] update available: current=%s latest=%s", strings.TrimSpace(a.Version), latest)
	}
	return data
}
