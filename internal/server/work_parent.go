package server

import (
	"database/sql"
	"fmt"
)

// validateWorkParent checks that parentID belongs to the same user, is not the work itself,
// and does not introduce a cycle when workID is the row being updated (workID 0 = create).
func (a *App) validateWorkParent(userID, workID, parentID int) error {
	if parentID <= 0 {
		return fmt.Errorf("invalid_parent")
	}
	if workID != 0 && parentID == workID {
		return fmt.Errorf("self_parent")
	}
	var owner int
	err := a.DB.QueryRow(`SELECT user_id FROM works WHERE id = ?`, parentID).Scan(&owner)
	if err != nil || owner != userID {
		return fmt.Errorf("parent_not_found")
	}
	if workID == 0 {
		return nil
	}
	cur := parentID
	for i := 0; i < 50; i++ {
		var pid sql.NullInt64
		if err := a.DB.QueryRow(`SELECT parent_work_id FROM works WHERE id = ?`, cur).Scan(&pid); err != nil {
			return nil
		}
		if !pid.Valid || pid.Int64 == 0 {
			return nil
		}
		p := int(pid.Int64)
		if p == workID {
			return fmt.Errorf("cycle")
		}
		cur = p
	}
	return fmt.Errorf("parent_chain_too_deep")
}
