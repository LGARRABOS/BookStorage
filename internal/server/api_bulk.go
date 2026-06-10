package server

import (
	"fmt"
	"net/http"
	"strings"
)

const maxBulkWorksIDs = 200

type bulkWorksRequest struct {
	IDs         []int            `json:"ids"`
	Patch       map[string]any   `json:"patch"`
	LinkReplace *bulkLinkReplace `json:"link_replace"`
	Delete      bool             `json:"delete"`
}

type bulkLinkReplace struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type bulkWorkError struct {
	ID    int    `json:"id"`
	Error string `json:"error"`
}

func (a *App) readingSiteOwnedBy(userID int, siteID int64) bool {
	var n int
	err := a.DB.QueryRow(`SELECT 1 FROM reading_sites WHERE id = ? AND user_id = ?`, siteID, userID).Scan(&n)
	return err == nil
}

func (a *App) HandleAPIWorksBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	userID, _ := a.currentUserID(r)

	var req bulkWorksRequest
	if err := decodeAPIJSONBody(w, r, &req); err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if len(req.IDs) == 0 {
		a.apiWriteError(w, http.StatusBadRequest, "ids_required")
		return
	}
	if len(req.IDs) > maxBulkWorksIDs {
		a.apiWriteError(w, http.StatusBadRequest, "too_many_ids")
		return
	}

	if req.LinkReplace != nil {
		if strings.TrimSpace(req.LinkReplace.From) == "" {
			a.apiWriteError(w, http.StatusBadRequest, "link_replace_from_required")
			return
		}
	}

	if !req.Delete && len(req.Patch) == 0 && req.LinkReplace == nil {
		a.apiWriteError(w, http.StatusBadRequest, "no_operation")
		return
	}

	var updated int
	var errs []bulkWorkError

	for _, workID := range req.IDs {
		if workID <= 0 {
			errs = append(errs, bulkWorkError{ID: workID, Error: "invalid_id"})
			continue
		}

		if req.Delete {
			result, err := a.DB.Exec(`DELETE FROM works WHERE id = ? AND user_id = ?`, workID, userID)
			if err != nil {
				errs = append(errs, bulkWorkError{ID: workID, Error: "internal_error"})
				continue
			}
			n, _ := result.RowsAffected()
			if n == 0 {
				errs = append(errs, bulkWorkError{ID: workID, Error: "not_found"})
				continue
			}
			updated++
			continue
		}

		setParts, args, buildErr := a.buildBulkWorkPatch(userID, workID, req.Patch)
		if buildErr != nil {
			errs = append(errs, bulkWorkError{ID: workID, Error: buildErr.Error()})
			continue
		}

		if req.LinkReplace != nil {
			setParts = append(setParts, "link = REPLACE(link, ?, ?)")
			args = append(args, req.LinkReplace.From, req.LinkReplace.To)
		}

		if len(setParts) == 0 {
			errs = append(errs, bulkWorkError{ID: workID, Error: "no_fields_to_update"})
			continue
		}

		setParts = append(setParts, "updated_at = CURRENT_TIMESTAMP")
		args = append(args, workID, userID)
		stmt := "UPDATE works SET " + strings.Join(setParts, ", ") + " WHERE id = ? AND user_id = ?"
		result, err := a.DB.Exec(stmt, args...)
		if err != nil {
			errs = append(errs, bulkWorkError{ID: workID, Error: "internal_error"})
			continue
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			errs = append(errs, bulkWorkError{ID: workID, Error: "not_found"})
			continue
		}
		updated++
	}

	a.apiWriteJSON(w, http.StatusOK, map[string]any{
		"updated": updated,
		"errors":  errs,
	})
}

func (a *App) buildBulkWorkPatch(userID, workID int, patch map[string]any) (setParts []string, args []any, err error) {
	if patch == nil {
		return nil, nil, nil
	}

	if v, ok := patch["status"].(string); ok && v != "" {
		newStatus := normalizeStatusForWrite(v)
		setParts = append(setParts, "status = ?")
		args = append(args, newStatus)
		var oldStatus string
		var startedAtNull, finishedAtNull bool
		qErr := a.DB.QueryRow(`SELECT COALESCE(status, ''), (started_at IS NULL), (finished_at IS NULL) FROM works WHERE id = ? AND user_id = ?`, workID, userID).Scan(&oldStatus, &startedAtNull, &finishedAtNull)
		if qErr == nil {
			if newStatus == "En cours" && oldStatus != "En cours" && startedAtNull {
				setParts = append(setParts, "started_at = CURRENT_TIMESTAMP")
			}
			if newStatus == "Terminé" && oldStatus != "Terminé" && finishedAtNull {
				setParts = append(setParts, "finished_at = CURRENT_TIMESTAMP")
			}
		}
	}

	if v, ok := patch["reading_type"].(string); ok && v != "" {
		setParts = append(setParts, "reading_type = ?")
		args = append(args, normalizeReadingTypeForWrite(v))
	}

	if raw, ok := patch["reading_site_id"]; ok {
		switch v := raw.(type) {
		case nil:
			setParts = append(setParts, "reading_site_id = NULL")
		case float64:
			siteID := int64(v)
			if siteID <= 0 {
				setParts = append(setParts, "reading_site_id = NULL")
			} else if !a.readingSiteOwnedBy(userID, siteID) {
				return nil, nil, fmt.Errorf("invalid_reading_site")
			} else {
				setParts = append(setParts, "reading_site_id = ?")
				args = append(args, siteID)
			}
		default:
			return nil, nil, fmt.Errorf("invalid_reading_site")
		}
	}

	if raw, ok := patch["notify_new_chapters"]; ok {
		var st string
		_ = a.DB.QueryRow(`SELECT COALESCE(status, '') FROM works WHERE id = ? AND user_id = ?`, workID, userID).Scan(&st)
		effStatus := normalizeStatusForWrite(st)
		if v, ok := patch["status"].(string); ok && strings.TrimSpace(v) != "" {
			effStatus = normalizeStatusForWrite(v)
		}
		switch v := raw.(type) {
		case bool:
			setParts = append(setParts, "notify_new_chapters = ?")
			args = append(args, notifyNewChaptersDB(effStatus, v))
		case float64:
			setParts = append(setParts, "notify_new_chapters = ?")
			args = append(args, notifyNewChaptersDB(effStatus, v != 0))
		}
	}

	if v, ok := patch["rating"].(float64); ok {
		setParts = append(setParts, "rating = ?")
		args = append(args, clampRating(int(v)))
	}

	return setParts, args, nil
}
