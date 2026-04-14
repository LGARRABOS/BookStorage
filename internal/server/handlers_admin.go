package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const adminDatabaseMaxRows = 400
const adminDatabaseMaxCellRunes = 160

// adminDatabaseSection is one table block for the admin DB overview (no secrets).
type adminDatabaseSection struct {
	Table          string
	HintKey        string
	Columns        []string
	Rows           [][]string
	Total          int
	Truncated      bool
	AllowRowDelete bool // single-column PK "id" shown first; dangerous tables excluded
}

var adminDatabaseTableSpecs = []struct {
	Name  string
	Order string
	Hint  string
}{
	{Name: "users", Order: "id DESC", Hint: "admin.database.hint.users"},
	{Name: "works", Order: "id DESC", Hint: "admin.database.hint.works"},
	{Name: "catalog", Order: "id DESC", Hint: "admin.database.hint.catalog"},
	{Name: "dismissed_recommendations", Order: "id DESC", Hint: "admin.database.hint.dismissed"},
	{Name: "sessions", Order: "id DESC", Hint: "admin.database.hint.sessions"},
	{Name: "translation_cache", Order: "created_at DESC", Hint: "admin.database.hint.translation_cache"},
	{Name: "csv_import_sessions", Order: "created_at DESC", Hint: "admin.database.hint.csv_import"},
	{Name: "schema_migrations", Order: "version DESC", Hint: "admin.database.hint.schema_migrations"},
}

func adminDatabaseOmitColumn(table, col string) bool {
	c := strings.ToLower(strings.TrimSpace(col))
	switch c {
	case "password", "token_hash":
		return true
	}
	if strings.EqualFold(table, "users") && c == "username" {
		return true
	}
	return false
}

func adminDatabaseCellString(col string, v any) string {
	if v == nil {
		return ""
	}
	var s string
	switch x := v.(type) {
	case []byte:
		s = string(x)
	case string:
		s = x
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		s = fmt.Sprint(x)
	}
	if strings.EqualFold(col, "raw_csv") {
		n := utf8.RuneCountInString(s)
		const preview = 72
		runes := []rune(s)
		if len(runes) > preview {
			return fmt.Sprintf("(%d runes) %s…", n, string(runes[:preview]))
		}
		return s
	}
	runes := []rune(s)
	if len(runes) > adminDatabaseMaxCellRunes {
		return string(runes[:adminDatabaseMaxCellRunes]) + "…"
	}
	return s
}

func buildAdminDatabaseSections(db *sql.DB) ([]adminDatabaseSection, error) {
	out := make([]adminDatabaseSection, 0, len(adminDatabaseTableSpecs))
	for _, spec := range adminDatabaseTableSpecs {
		sec, err := scanAdminDatabaseTable(db, spec.Name, spec.Order, spec.Hint)
		if err != nil {
			return nil, fmt.Errorf("table %s: %w", spec.Name, err)
		}
		sec.AllowRowDelete = adminDatabaseTableAllowsRowDelete(spec.Name, sec.Columns)
		out = append(out, sec)
	}
	return out, nil
}

var adminDatabaseIDNumeric = regexp.MustCompile(`^[0-9]+$`)
var adminDatabaseIDSessionCSV = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

func adminDatabaseTableAllowsRowDelete(table string, columns []string) bool {
	if len(columns) == 0 || !strings.EqualFold(columns[0], "id") {
		return false
	}
	switch strings.ToLower(table) {
	case "works", "catalog", "dismissed_recommendations", "sessions", "csv_import_sessions":
		return true
	default:
		return false
	}
}

func adminDatabaseDeleteTableOK(table string) bool {
	switch strings.ToLower(strings.TrimSpace(table)) {
	case "works", "catalog", "dismissed_recommendations", "sessions", "csv_import_sessions":
		return true
	default:
		return false
	}
}

// HandleAPIAdminDatabaseDelete removes one row by primary key "id" (allowed tables only).
func (a *App) HandleAPIAdminDatabaseDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Table string `json:"table"`
		ID    string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	req.Table = strings.TrimSpace(req.Table)
	req.ID = strings.TrimSpace(req.ID)
	if req.Table == "" || req.ID == "" {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !adminDatabaseDeleteTableOK(req.Table) {
		a.apiWriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	switch req.Table {
	case "csv_import_sessions":
		if !adminDatabaseIDSessionCSV.MatchString(req.ID) {
			a.apiWriteError(w, http.StatusBadRequest, "invalid_id")
			return
		}
	default:
		if !adminDatabaseIDNumeric.MatchString(req.ID) {
			a.apiWriteError(w, http.StatusBadRequest, "invalid_id")
			return
		}
	}

	q := `DELETE FROM ` + quoteSQLiteIdent(req.Table) + ` WHERE id = ?`
	res, err := a.DB.Exec(q, req.ID)
	if err != nil {
		log.Printf("admin database delete %s id=%s: %v", req.Table, req.ID, err)
		a.apiWriteError(w, http.StatusBadRequest, "delete_failed")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		a.apiWriteError(w, http.StatusNotFound, "not_found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "deleted": n})
}

func scanAdminDatabaseTable(db *sql.DB, table, orderSQL, hintKey string) (adminDatabaseSection, error) {
	sec := adminDatabaseSection{Table: table, HintKey: hintKey}
	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&exists); err != nil {
		return sec, err
	}
	if exists == 0 {
		return sec, nil
	}

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + quoteSQLiteIdent(table)).Scan(&total); err != nil {
		return sec, err
	}
	sec.Total = total

	q := `SELECT * FROM ` + quoteSQLiteIdent(table)
	if strings.TrimSpace(orderSQL) != "" {
		q += ` ORDER BY ` + orderSQL
	}
	q += ` LIMIT ?`
	rows, err := db.Query(q, adminDatabaseMaxRows)
	if err != nil {
		return sec, err
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return sec, err
	}
	keepIdx := make([]int, 0, len(cols))
	for i, c := range cols {
		if !adminDatabaseOmitColumn(table, c) {
			keepIdx = append(keepIdx, i)
			sec.Columns = append(sec.Columns, c)
		}
	}

	n := 0
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return sec, err
		}
		row := make([]string, len(keepIdx))
		for j, i := range keepIdx {
			row[j] = adminDatabaseCellString(cols[i], raw[i])
		}
		sec.Rows = append(sec.Rows, row)
		n++
	}
	if err := rows.Err(); err != nil {
		return sec, err
	}
	sec.Truncated = total > n
	return sec, nil
}

// quoteSQLiteIdent wraps a known-safe table name for SQLite.
func quoteSQLiteIdent(name string) string {
	if name == "" || strings.ContainsAny(name, `"';\x00`) {
		return `""`
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (a *App) HandleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query(
		`SELECT id, username, password, validated, is_admin, is_superadmin,
                display_name, email, bio, avatar_path, is_public
         FROM users`,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()

	type adminUser struct {
		ID           int
		Username     string
		Validated    int
		IsAdmin      int
		IsSuperadmin int
		DisplayName  sql.NullString
		Email        sql.NullString
		Bio          sql.NullString
		AvatarPath   sql.NullString
		IsPublic     sql.NullInt64
	}

	var users []adminUser
	for rows.Next() {
		var u adminUser
		var pwd string
		if err := rows.Scan(
			&u.ID,
			&u.Username,
			&pwd,
			&u.Validated,
			&u.IsAdmin,
			&u.IsSuperadmin,
			&u.DisplayName,
			&u.Email,
			&u.Bio,
			&u.AvatarPath,
			&u.IsPublic,
		); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}

	a.renderTemplate(w, r, "admin_accounts", a.mergeData(r, map[string]any{
		"Users": users,
	}))
}

func (a *App) HandleApproveAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := strconv.Atoi(r.PathValue("id"))
	if _, err := a.DB.Exec(
		`UPDATE users SET validated = 1 WHERE id = ?`,
		userID,
	); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/accounts", http.StatusFound)
}

func (a *App) HandleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	targetID, _ := strconv.Atoi(r.PathValue("id"))

	var isAdmin, isSuper int
	err := a.DB.QueryRow(
		`SELECT is_admin, is_superadmin FROM users WHERE id = ?`,
		targetID,
	).Scan(&isAdmin, &isSuper)
	if err != nil {
		http.Redirect(w, r, "/admin/accounts", http.StatusFound)
		return
	}

	if isSuper != 0 {
		http.Redirect(w, r, "/admin/accounts", http.StatusFound)
		return
	}

	if _, err := a.DB.Exec(`DELETE FROM users WHERE id = ?`, targetID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/accounts", http.StatusFound)
}

func (a *App) HandlePromoteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	targetID, _ := strconv.Atoi(r.PathValue("id"))
	if _, err := a.DB.Exec(
		`UPDATE users SET is_admin = 1, validated = 1 WHERE id = ?`,
		targetID,
	); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/accounts", http.StatusFound)
}

func (a *App) HandleAdminMonitoring(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	summary := FetchPrometheusAdminSummary(a.Settings)
	a.renderTemplate(w, r, "admin_monitoring", a.mergeData(r, map[string]any{
		"PrometheusSummary": summary,
	}))
}

func (a *App) HandleAdminDatabase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	sections, err := buildAdminDatabaseSections(a.DB)
	if err != nil {
		log.Printf("admin database overview: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	a.renderTemplate(w, r, "admin_database", a.mergeData(r, map[string]any{
		"DBSections": sections,
	}))
}

func (a *App) HandleAPIAdminPrometheusSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s := FetchPrometheusAdminSummary(a.Settings)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"reachable":       s.Reachable,
		"query_base":      s.QueryBase,
		"up":              s.Up,
		"scrape_ok":       s.ScrapeJobHealthy,
		"requests_total":  s.RequestsTotal,
		"request_rate_5m": s.RequestRate5m,
		"requests_2xx":    s.Requests2xx,
		"requests_3xx":    s.Requests3xx,
		"requests_4xx":    s.Requests4xx,
		"requests_5xx":    s.Requests5xx,
		"requests_get":    s.RequestsGet,
		"requests_post":   s.RequestsPost,
		"error_rate_5m":   s.ErrorRate5m,
		"latency_p50":     s.LatencyP50,
		"latency_p95":     s.LatencyP95,
		"error":           s.Error,
		"invalid_url":     s.Error == "invalid_prometheus_url",
	})
}

func (a *App) HandleAdminUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	latestTag, latestInfo, latestOK := a.computeUpdateTag(updateModeLatest)
	majorTag, majorInfo, majorOK := a.computeUpdateTag(updateModeLatestMajor)

	a.renderTemplate(w, r, "admin_update", a.mergeData(r, map[string]any{
		"LatestTag":       latestTag,
		"LatestTagOK":     latestOK,
		"LatestInfo":      latestInfo,
		"LatestMajorTag":  majorTag,
		"LatestMajorOK":   majorOK,
		"LatestMajorInfo": majorInfo,
	}))
}

func (a *App) HandleAPIUpdateLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	res := a.triggerUpdate(r.Context(), updateModeLatest)
	w.Header().Set("Content-Type", "application/json")
	if !res.OK {
		w.WriteHeader(http.StatusBadGateway)
	}
	_ = json.NewEncoder(w).Encode(res)
}

func (a *App) HandleAPIUpdateLatestMajor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	res := a.triggerUpdate(r.Context(), updateModeLatestMajor)
	w.Header().Set("Content-Type", "application/json")
	if !res.OK {
		w.WriteHeader(http.StatusBadGateway)
	}
	_ = json.NewEncoder(w).Encode(res)
}

func (a *App) HandleAPIUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.updateStatus())
}
