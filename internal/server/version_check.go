package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	versionCheckTimeout    = 8 * time.Second
	versionCheckCacheOK    = 45 * time.Minute
	versionCheckCacheFail  = 5 * time.Minute
	githubAPIVersionHeader = "2022-11-28"
)

var (
	githubReleasesLatestURL = "https://api.github.com/repos/LGARRABOS/BookStorage/releases/latest"
	githubReleasesListURL   = "https://api.github.com/repos/LGARRABOS/BookStorage/releases?per_page=10"
)

type githubRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

type versionCheckCache struct {
	mu        sync.RWMutex
	fetchedAt time.Time
	ok        bool
	tagName   string
	htmlURL   string
}

var releaseCheckCache versionCheckCache

var versionCheckHTTPClient = &http.Client{Timeout: versionCheckTimeout}

// runningVersion returns the semver used for update checks (build ldflags or BOOKSTORAGE_APP_VERSION).
func (a *App) runningVersion() string {
	v := strings.TrimSpace(a.Version)
	if v != "" && !strings.EqualFold(v, "dev") {
		if _, _, _, ok := parseSemver(v); ok {
			return v
		}
	}
	if env := strings.TrimSpace(os.Getenv("BOOKSTORAGE_APP_VERSION")); env != "" {
		return env
	}
	return v
}

func githubAuthToken() string {
	for _, key := range []string{"BOOKSTORAGE_GITHUB_TOKEN", "GITHUB_TOKEN"} {
		if t := strings.TrimSpace(os.Getenv(key)); t != "" {
			return t
		}
	}
	return ""
}

func applyGitHubHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "BookStorage")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersionHeader)
	if token := githubAuthToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func decodeRelease(body []byte) (tagName, htmlURL string, ok bool) {
	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", "", false
	}
	if rel.Draft || rel.Prerelease {
		return "", "", false
	}
	tagName = strings.TrimSpace(rel.TagName)
	if tagName == "" {
		return "", "", false
	}
	return tagName, strings.TrimSpace(rel.HTMLURL), true
}

func fetchReleaseGET(client *http.Client, url string) (tagName, htmlURL string, ok bool) {
	if client == nil {
		client = versionCheckHTTPClient
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", "", false
	}
	applyGitHubHeaders(req)

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
	return decodeRelease(body)
}

func fetchLatestReleaseList(client *http.Client) (tagName, htmlURL string, ok bool) {
	if client == nil {
		client = versionCheckHTTPClient
	}
	req, err := http.NewRequest(http.MethodGet, githubReleasesListURL, nil)
	if err != nil {
		return "", "", false
	}
	applyGitHubHeaders(req)

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
	var releases []githubRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return "", "", false
	}
	var bestTag, bestURL string
	for _, rel := range releases {
		if rel.Draft || rel.Prerelease {
			continue
		}
		tag := strings.TrimSpace(rel.TagName)
		if tag == "" {
			continue
		}
		if bestTag == "" || semverCompare(tag, bestTag) > 0 {
			bestTag = tag
			bestURL = strings.TrimSpace(rel.HTMLURL)
		}
	}
	if bestTag == "" {
		return "", "", false
	}
	return bestTag, bestURL, true
}

func fetchLatestRelease(client *http.Client) (tagName, htmlURL string, ok bool) {
	tag, url, ok := fetchReleaseGET(client, githubReleasesLatestURL)
	if ok {
		return tag, url, true
	}
	return fetchLatestReleaseList(client)
}

func (c *versionCheckCache) get() (tagName, htmlURL string, ok, fresh bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.fetchedAt.IsZero() {
		return "", "", false, false
	}
	ttl := versionCheckCacheOK
	if !c.ok {
		ttl = versionCheckCacheFail
	}
	if time.Since(c.fetchedAt) > ttl {
		return "", "", false, false
	}
	return c.tagName, c.htmlURL, c.ok, true
}

func (c *versionCheckCache) set(tagName, htmlURL string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fetchedAt = time.Now()
	c.ok = ok
	c.tagName = tagName
	c.htmlURL = htmlURL
}

func cachedLatestRelease(client *http.Client) (tagName, htmlURL string, ok bool) {
	if tag, url, hit, fresh := releaseCheckCache.get(); fresh {
		return tag, url, hit
	}
	tagName, htmlURL, ok = fetchLatestRelease(client)
	releaseCheckCache.set(tagName, htmlURL, ok)
	if !ok {
		log.Printf("[admin] GitHub release check failed (latest=%q)", githubReleasesLatestURL)
	}
	return tagName, htmlURL, ok
}

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

type updateCheckResult struct {
	available     bool
	latestVersion string
	releaseURL    string
	skippedReason string // "dev", "invalid_version", "github_unreachable"
}

// checkUpdateAvailable compares the running version with the latest GitHub release.
func (a *App) checkUpdateAvailable() updateCheckResult {
	current := a.runningVersion()
	if current == "" || strings.EqualFold(current, "dev") {
		return updateCheckResult{skippedReason: "dev"}
	}
	if _, _, _, ok := parseSemver(current); !ok {
		return updateCheckResult{skippedReason: "invalid_version"}
	}

	tag, url, ok := cachedLatestRelease(versionCheckHTTPClient)
	if !ok {
		return updateCheckResult{skippedReason: "github_unreachable"}
	}
	if semverCompare(current, tag) >= 0 {
		return updateCheckResult{}
	}
	return updateCheckResult{
		available:     true,
		latestVersion: tag,
		releaseURL:    url,
	}
}

func (a *App) isSuperadminRequest(r *http.Request) bool {
	if a == nil || a.DB == nil || r == nil {
		return false
	}
	uid, ok := a.currentUserID(r)
	if !ok {
		return false
	}
	var sup int
	if err := a.DB.QueryRow(`SELECT is_superadmin FROM users WHERE id = ?`, uid).Scan(&sup); err != nil {
		return false
	}
	return sup != 0
}

// adminUpdateData returns template fields for the admin update banner.
func (a *App) adminUpdateData(r *http.Request) map[string]any {
	res := a.checkUpdateAvailable()
	data := map[string]any{
		"UpdateAvailable": res.available,
		"AppVersion":      a.runningVersion(),
	}
	if res.available {
		data["LatestVersion"] = res.latestVersion
		if res.releaseURL != "" {
			data["ReleaseURL"] = res.releaseURL
		}
		log.Printf("[admin] update available: current=%s latest=%s", a.runningVersion(), res.latestVersion)
		return data
	}
	if r != nil && a.isSuperadminRequest(r) {
		switch res.skippedReason {
		case "dev":
			data["UpdateCheckDevBuild"] = true
		case "invalid_version":
			data["UpdateCheckInvalidVersion"] = true
		case "github_unreachable":
			data["UpdateCheckUnreachable"] = true
		}
	}
	return data
}
