package db

import (
	"database/sql"
	"fmt"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS series (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    title      TEXT NOT NULL,
    dir_path   TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS episodes (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    series_id               INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    file_path               TEXT NOT NULL UNIQUE,
    file_name               TEXT NOT NULL,
    episode_number          INTEGER,
    size_bytes              INTEGER,
    mod_time                DATETIME,
    status                  TEXT NOT NULL DEFAULT 'new' CHECK(status IN ('new','watching','watched')),
    started_at              DATETIME,
    finished_at             DATETIME,
    resume_position_seconds REAL,
    duration_seconds        REAL
);

CREATE INDEX IF NOT EXISTS idx_episodes_series ON episodes(series_id);
CREATE INDEX IF NOT EXISTS idx_episodes_status ON episodes(status);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`

// addedColumns lists columns introduced after the initial schema, for
// databases created before they existed. CREATE TABLE IF NOT EXISTS won't
// retrofit these onto an already-existing table, so they're added here
// idempotently instead of via a full migration framework.
var addedColumns = map[string][2]string{
	"resume_position_seconds": {"episodes", "REAL"},
	"duration_seconds":        {"episodes", "REAL"},
}

func migrate(conn *sql.DB) error {
	if _, err := conn.Exec(schemaSQL); err != nil {
		return err
	}
	return addMissingColumns(conn)
}

func addMissingColumns(conn *sql.DB) error {
	for column, tableAndType := range addedColumns {
		table, sqlType := tableAndType[0], tableAndType[1]
		exists, err := hasColumn(conn, table, column)
		if err != nil {
			return fmt.Errorf("checking column %s.%s: %w", table, column, err)
		}
		if exists {
			continue
		}
		if _, err := conn.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, sqlType)); err != nil {
			return fmt.Errorf("adding column %s.%s: %w", table, column, err)
		}
	}
	return nil
}

func hasColumn(conn *sql.DB, table, column string) (bool, error) {
	rows, err := conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
