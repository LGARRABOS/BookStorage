package server

import (
	"fmt"

	"bookstorage/internal/database"
)

// deleteUserOwnedData removes every row that references userID, then the user itself.
// Callers must run this inside a transaction; FK order avoids constraint failures
// on both SQLite (PRAGMA foreign_keys=ON) and PostgreSQL.
func deleteUserOwnedData(tx *database.Tx, userID int) error {
	if tx == nil {
		return fmt.Errorf("delete user data: nil transaction")
	}
	if userID <= 0 {
		return fmt.Errorf("delete user data: invalid user id")
	}
	stmts := []string{
		`DELETE FROM webhook_deliveries WHERE endpoint_id IN (SELECT id FROM webhook_endpoints WHERE user_id = ?)`,
		`DELETE FROM webhook_endpoints WHERE user_id = ?`,
		`DELETE FROM library_placements WHERE user_id = ?`,
		`DELETE FROM library_shelves WHERE furniture_id IN (SELECT id FROM library_furniture WHERE user_id = ?)`,
		`DELETE FROM library_furniture WHERE user_id = ?`,
		`UPDATE works SET parent_work_id = NULL WHERE user_id = ?`,
		`DELETE FROM works WHERE user_id = ?`,
		`DELETE FROM anime_works WHERE user_id = ?`,
		`DELETE FROM bd_works WHERE user_id = ?`,
		`DELETE FROM manga_phys_works WHERE user_id = ?`,
		`DELETE FROM dismissed_recommendations WHERE user_id = ?`,
		`DELETE FROM sessions WHERE user_id = ?`,
		`DELETE FROM csv_import_sessions WHERE user_id = ?`,
		`DELETE FROM reading_activity_daily WHERE user_id = ?`,
		`DELETE FROM api_tokens WHERE user_id = ?`,
		`DELETE FROM user_catalog_blocklist WHERE user_id = ?`,
		`DELETE FROM webauthn_credentials WHERE user_id = ?`,
		`DELETE FROM webauthn_challenges WHERE user_id = ?`,
		`DELETE FROM password_reset_tokens WHERE user_id = ?`,
		`DELETE FROM oauth_states WHERE user_id = ?`,
		`DELETE FROM reading_sites WHERE user_id = ?`,
		`DELETE FROM admin_audit_log WHERE actor_user_id = ?`,
		`DELETE FROM login_attempts WHERE username = (SELECT username FROM users WHERE id = ?)`,
		`DELETE FROM users WHERE id = ?`,
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q, userID); err != nil {
			return err
		}
	}
	return nil
}
