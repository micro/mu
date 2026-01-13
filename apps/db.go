package apps

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mu/app"

	_ "modernc.org/sqlite"
)

const (
	// MaxQuotaBytes is the maximum storage per app per user (100KB)
	MaxQuotaBytes = 100 * 1024
)

var (
	db     *sql.DB
	dbOnce sync.Once
)

// initDB initializes the apps database
func initDB() error {
	var initErr error
	dbOnce.Do(func() {
		dir := os.ExpandEnv("$HOME/.mu")
		dbPath := filepath.Join(dir, "apps.db")
		os.MkdirAll(dir, 0700)

		var err error
		db, err = sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
		if err != nil {
			initErr = err
			return
		}

		// Create tables
		_, err = db.Exec(`
			CREATE TABLE IF NOT EXISTS app_data (
				app_id TEXT NOT NULL,
				user_id TEXT NOT NULL,
				key TEXT NOT NULL,
				value BLOB,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (app_id, user_id, key)
			);

			CREATE TABLE IF NOT EXISTS app_quota (
				app_id TEXT NOT NULL,
				user_id TEXT NOT NULL,
				bytes_used INTEGER DEFAULT 0,
				PRIMARY KEY (app_id, user_id)
			);

			CREATE INDEX IF NOT EXISTS idx_app_data_app_user ON app_data(app_id, user_id);
		`)
		if err != nil {
			initErr = err
			return
		}

		app.Log("apps", "Initialized apps.db")
	})
	return initErr
}

// DBGet retrieves a value for an app/user/key
func DBGet(appID, userID, key string) (interface{}, error) {
	if err := initDB(); err != nil {
		return nil, err
	}

	var valueBytes []byte
	err := db.QueryRow(
		"SELECT value FROM app_data WHERE app_id = ? AND user_id = ? AND key = ?",
		appID, userID, key,
	).Scan(&valueBytes)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var value interface{}
	if err := json.Unmarshal(valueBytes, &value); err != nil {
		return nil, err
	}

	return value, nil
}

// DBSet stores a value for an app/user/key
func DBSet(appID, userID, key string, value interface{}) error {
	if err := initDB(); err != nil {
		return err
	}

	valueBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}

	// Check quota
	var currentBytes int64
	err = db.QueryRow(
		"SELECT bytes_used FROM app_quota WHERE app_id = ? AND user_id = ?",
		appID, userID,
	).Scan(&currentBytes)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	// Get existing value size (if any)
	var existingSize int64
	var existingBytes []byte
	err = db.QueryRow(
		"SELECT value FROM app_data WHERE app_id = ? AND user_id = ? AND key = ?",
		appID, userID, key,
	).Scan(&existingBytes)
	if err == nil {
		existingSize = int64(len(existingBytes))
	}

	// Calculate new total
	newTotal := currentBytes - existingSize + int64(len(valueBytes))
	if newTotal > MaxQuotaBytes {
		return fmt.Errorf("quota exceeded: %d bytes used, %d bytes limit", newTotal, MaxQuotaBytes)
	}

	// Upsert the value
	_, err = db.Exec(`
		INSERT INTO app_data (app_id, user_id, key, value, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(app_id, user_id, key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`, appID, userID, key, valueBytes, time.Now())
	if err != nil {
		return err
	}

	// Update quota
	_, err = db.Exec(`
		INSERT INTO app_quota (app_id, user_id, bytes_used)
		VALUES (?, ?, ?)
		ON CONFLICT(app_id, user_id) DO UPDATE SET
			bytes_used = ?
	`, appID, userID, newTotal, newTotal)

	return err
}

// DBDelete removes a value for an app/user/key
func DBDelete(appID, userID, key string) error {
	if err := initDB(); err != nil {
		return err
	}

	// Get existing value size
	var existingBytes []byte
	err := db.QueryRow(
		"SELECT value FROM app_data WHERE app_id = ? AND user_id = ? AND key = ?",
		appID, userID, key,
	).Scan(&existingBytes)
	if err == sql.ErrNoRows {
		return nil // Nothing to delete
	}
	if err != nil {
		return err
	}

	// Delete the value
	_, err = db.Exec(
		"DELETE FROM app_data WHERE app_id = ? AND user_id = ? AND key = ?",
		appID, userID, key,
	)
	if err != nil {
		return err
	}

	// Update quota
	_, err = db.Exec(`
		UPDATE app_quota SET bytes_used = bytes_used - ?
		WHERE app_id = ? AND user_id = ?
	`, len(existingBytes), appID, userID)

	return err
}

// DBList returns all keys for an app/user
func DBList(appID, userID string) ([]string, error) {
	if err := initDB(); err != nil {
		return nil, err
	}

	rows, err := db.Query(
		"SELECT key FROM app_data WHERE app_id = ? AND user_id = ? ORDER BY key",
		appID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	return keys, nil
}

// DBQuota returns bytes used and limit for an app/user
func DBQuota(appID, userID string) (used int64, limit int64, err error) {
	if err := initDB(); err != nil {
		return 0, MaxQuotaBytes, err
	}

	err = db.QueryRow(
		"SELECT bytes_used FROM app_quota WHERE app_id = ? AND user_id = ?",
		appID, userID,
	).Scan(&used)
	if err == sql.ErrNoRows {
		return 0, MaxQuotaBytes, nil
	}

	return used, MaxQuotaBytes, err
}
