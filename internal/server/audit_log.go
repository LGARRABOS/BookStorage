package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

type auditLogEntry struct {
	ID          int
	ActorUserID int
	ActorName   string
	Action      string
	TargetType  sql.NullString
	TargetID    sql.NullString
	DetailJSON  sql.NullString
	IP          sql.NullString
	CreatedAt   string
}

func (a *App) logAdminAction(r *http.Request, action, targetType, targetID string, detail any) {
	actorID, ok := a.currentUserID(r)
	if !ok || actorID <= 0 {
		return
	}
	var detailArg any
	if detail != nil {
		b, err := json.Marshal(detail)
		if err == nil {
			detailArg = string(b)
		}
	}
	trustProxy := a.Settings != nil && a.Settings.TrustProxy
	ip := clientIP(r, trustProxy)
	var targetTypeArg, targetIDArg any
	if targetType != "" {
		targetTypeArg = targetType
	}
	if targetID != "" {
		targetIDArg = targetID
	}
	_, _ = a.DB.Exec(
		`INSERT INTO admin_audit_log (actor_user_id, action, target_type, target_id, detail_json, ip) VALUES (?, ?, ?, ?, ?, ?)`,
		actorID, action, targetTypeArg, targetIDArg, detailArg, ip,
	)
}

func (a *App) listAuditLog(limit int) ([]auditLogEntry, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := a.DB.Query(
		`SELECT l.id, l.actor_user_id, COALESCE(u.username, ''), l.action, l.target_type, l.target_id, l.detail_json, l.ip, l.created_at
		 FROM admin_audit_log l
		 LEFT JOIN users u ON u.id = l.actor_user_id
		 ORDER BY l.id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []auditLogEntry
	for rows.Next() {
		var e auditLogEntry
		var created any
		if err := rows.Scan(&e.ID, &e.ActorUserID, &e.ActorName, &e.Action, &e.TargetType, &e.TargetID, &e.DetailJSON, &e.IP, &created); err != nil {
			return nil, err
		}
		switch v := created.(type) {
		case time.Time:
			e.CreatedAt = v.UTC().Format("2006-01-02 15:04:05")
		case []byte:
			e.CreatedAt = string(v)
		case string:
			e.CreatedAt = v
		default:
			e.CreatedAt = ""
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (a *App) HandleAdminAuditLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	entries, err := a.listAuditLog(200)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	a.renderTemplate(w, r, "admin_audit", a.mergeData(r, map[string]any{
		"AuditEntries": entries,
	}))
}
