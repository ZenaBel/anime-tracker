package db

import "database/sql"

const schemaSQL = `
CREATE TABLE IF NOT EXISTS series (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    title      TEXT NOT NULL,
    dir_path   TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS episodes (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    series_id      INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    file_path      TEXT NOT NULL UNIQUE,
    file_name      TEXT NOT NULL,
    episode_number INTEGER,
    size_bytes     INTEGER,
    mod_time       DATETIME,
    status         TEXT NOT NULL DEFAULT 'new' CHECK(status IN ('new','watching','watched')),
    started_at     DATETIME,
    finished_at    DATETIME
);

CREATE INDEX IF NOT EXISTS idx_episodes_series ON episodes(series_id);
CREATE INDEX IF NOT EXISTS idx_episodes_status ON episodes(status);
`

func migrate(conn *sql.DB) error {
	_, err := conn.Exec(schemaSQL)
	return err
}
