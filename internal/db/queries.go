package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

const (
	StatusNew      = "new"
	StatusWatching = "watching"
	StatusWatched  = "watched"
)

type Episode struct {
	ID                 int64
	SeriesID           int64
	FilePath           string
	FileName           string
	EpisodeNumber      *int
	SizeBytes          int64
	ModTime            time.Time
	Status             string
	StartedAt          *time.Time
	FinishedAt         *time.Time
	ResumePositionSecs *float64
	DurationSecs       *float64
}

// ProgressPercent returns how far into the episode playback last got, 0-100,
// or false if no position/duration has ever been recorded.
func (e Episode) ProgressPercent() (int, bool) {
	if e.ResumePositionSecs == nil || e.DurationSecs == nil || *e.DurationSecs <= 0 {
		return 0, false
	}
	pct := int(*e.ResumePositionSecs / *e.DurationSecs * 100)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return pct, true
}

// FirstUnwatchedIndex returns the index of the first episode in eps that
// isn't fully watched (i.e. the next one to watch), or 0 if there is none
// or eps is empty.
func FirstUnwatchedIndex(eps []Episode) int {
	for i, ep := range eps {
		if ep.Status != StatusWatched {
			return i
		}
	}
	return 0
}

type SeriesProgress struct {
	ID      int64
	Title   string
	DirPath string
	Total   int
	Watched int
}

type SortMode int

const (
	SortAlphaAsc SortMode = iota
	SortAlphaDesc
	SortAdded
	SortLastWatched
)

var sortModeNames = map[SortMode]string{
	SortAlphaAsc:    "az",
	SortAlphaDesc:   "za",
	SortAdded:       "added",
	SortLastWatched: "watched",
}

func (m SortMode) String() string {
	if s, ok := sortModeNames[m]; ok {
		return s
	}
	return "az"
}

func ParseSortMode(s string) (SortMode, error) {
	switch s {
	case "", "az":
		return SortAlphaAsc, nil
	case "za":
		return SortAlphaDesc, nil
	case "added":
		return SortAdded, nil
	case "watched":
		return SortLastWatched, nil
	default:
		return SortAlphaAsc, fmt.Errorf("invalid sort %q (want az, za, added, or watched)", s)
	}
}

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

// SeriesIDByDirPath looks up a series by its directory path without
// creating one if it doesn't exist.
func (s *Store) SeriesIDByDirPath(ctx context.Context, dirPath string) (int64, bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM series WHERE dir_path = ?`, dirPath).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("querying series: %w", err)
	}
	return id, true, nil
}

// UpsertSeries ensures a series row exists for dirPath, updating its title
// if it changed. Returns the row id and whether it was newly created.
func (s *Store) UpsertSeries(ctx context.Context, title, dirPath string) (int64, bool, error) {
	var id int64
	var existingTitle string
	err := s.db.QueryRowContext(ctx, `SELECT id, title FROM series WHERE dir_path = ?`, dirPath).Scan(&id, &existingTitle)
	if errors.Is(err, sql.ErrNoRows) {
		res, err := s.db.ExecContext(ctx, `INSERT INTO series (title, dir_path) VALUES (?, ?)`, title, dirPath)
		if err != nil {
			return 0, false, fmt.Errorf("inserting series: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return 0, false, fmt.Errorf("getting new series id: %w", err)
		}
		return id, true, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("querying series: %w", err)
	}

	if existingTitle != title {
		if _, err := s.db.ExecContext(ctx, `UPDATE series SET title = ? WHERE id = ?`, title, id); err != nil {
			return 0, false, fmt.Errorf("updating series title: %w", err)
		}
	}
	return id, false, nil
}

// UpsertEpisodeSeen records that filePath was observed on disk. On an
// existing row it only refreshes metadata (size, mod time, episode number)
// and never touches status/timestamps. Returns whether the row was new.
func (s *Store) UpsertEpisodeSeen(ctx context.Context, seriesID int64, filePath, fileName string, epNum *int, size int64, modTime time.Time) (bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM episodes WHERE file_path = ?`, filePath).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO episodes (series_id, file_path, file_name, episode_number, size_bytes, mod_time, status)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			seriesID, filePath, fileName, epNum, size, modTime, StatusNew)
		if err != nil {
			return false, fmt.Errorf("inserting episode: %w", err)
		}
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("querying episode: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE episodes SET size_bytes = ?, mod_time = ?, episode_number = ?, file_name = ?
		WHERE id = ?`,
		size, modTime, epNum, fileName, id)
	if err != nil {
		return false, fmt.Errorf("updating episode: %w", err)
	}
	return false, nil
}

// MarkMissingAsWatched marks episodes of seriesID as watched if their
// file_path is not present in seenPaths and they aren't already watched.
func (s *Store) MarkMissingAsWatched(ctx context.Context, seriesID int64, seenPaths []string) (int64, error) {
	seen := make(map[string]struct{}, len(seenPaths))
	for _, p := range seenPaths {
		seen[p] = struct{}{}
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, file_path FROM episodes WHERE series_id = ? AND status != ?`, seriesID, StatusWatched)
	if err != nil {
		return 0, fmt.Errorf("querying episodes: %w", err)
	}

	var missingIDs []int64
	for rows.Next() {
		var id int64
		var filePath string
		if err := rows.Scan(&id, &filePath); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning episode: %w", err)
		}
		if _, ok := seen[filePath]; !ok {
			missingIDs = append(missingIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating episodes: %w", err)
	}
	rows.Close()

	var count int64
	for _, id := range missingIDs {
		_, err := s.db.ExecContext(ctx, `
			UPDATE episodes
			SET status = ?,
			    finished_at = COALESCE(finished_at, CURRENT_TIMESTAMP),
			    started_at = COALESCE(started_at, CURRENT_TIMESTAMP)
			WHERE id = ?`,
			StatusWatched, id)
		if err != nil {
			return count, fmt.Errorf("marking episode watched: %w", err)
		}
		count++
	}
	return count, nil
}

var sortOrderClauses = map[SortMode]string{
	SortAlphaAsc:  "ORDER BY series.title COLLATE NOCASE ASC",
	SortAlphaDesc: "ORDER BY series.title COLLATE NOCASE DESC",
	// "Added" means "most recently got new content", not "when the series
	// was first tracked" — otherwise a series that just got new episodes
	// scanned in wouldn't move, since its own series.id never changes.
	// episodes.id is a strictly increasing AUTOINCREMENT key, so the max
	// per series is a reliable "most recently added episode" proxy, and
	// it doubles as the original tie-breaker for series bulk-added within
	// the same created_at tick during one scan (their first episode's id
	// still orders correctly).
	SortAdded:       "ORDER BY MAX(episodes.id) IS NULL ASC, MAX(episodes.id) DESC",
	SortLastWatched: "ORDER BY MAX(episodes.finished_at) IS NULL ASC, MAX(episodes.finished_at) DESC",
}

func (s *Store) ListSeriesWithProgress(ctx context.Context, sort SortMode) ([]SeriesProgress, error) {
	orderClause, ok := sortOrderClauses[sort]
	if !ok {
		orderClause = sortOrderClauses[SortAlphaAsc]
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT series.id, series.title, series.dir_path,
		       COUNT(episodes.id) AS total,
		       COALESCE(SUM(CASE WHEN episodes.status = ? THEN 1 ELSE 0 END), 0) AS watched
		FROM series
		LEFT JOIN episodes ON episodes.series_id = series.id
		GROUP BY series.id
		`+orderClause, StatusWatched)
	if err != nil {
		return nil, fmt.Errorf("querying series progress: %w", err)
	}
	defer rows.Close()

	var out []SeriesProgress
	for rows.Next() {
		var sp SeriesProgress
		if err := rows.Scan(&sp.ID, &sp.Title, &sp.DirPath, &sp.Total, &sp.Watched); err != nil {
			return nil, fmt.Errorf("scanning series progress: %w", err)
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

// ListEpisodesBySeries returns episodes for a series, ordered by episode
// number (unparsed numbers last) then filename.
func (s *Store) ListEpisodesBySeries(ctx context.Context, seriesID int64) ([]Episode, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, series_id, file_path, file_name, episode_number, size_bytes, mod_time, status, started_at, finished_at, resume_position_seconds, duration_seconds
		FROM episodes
		WHERE series_id = ?
		ORDER BY episode_number IS NULL, episode_number, file_name`, seriesID)
	if err != nil {
		return nil, fmt.Errorf("querying episodes: %w", err)
	}
	defer rows.Close()

	var out []Episode
	for rows.Next() {
		ep, err := scanEpisode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ep)
	}
	return out, rows.Err()
}

func (s *Store) ListAllEpisodes(ctx context.Context) ([]Episode, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, series_id, file_path, file_name, episode_number, size_bytes, mod_time, status, started_at, finished_at, resume_position_seconds, duration_seconds
		FROM episodes
		ORDER BY episode_number IS NULL, episode_number, file_name`)
	if err != nil {
		return nil, fmt.Errorf("querying episodes: %w", err)
	}
	defer rows.Close()

	var out []Episode
	for rows.Next() {
		ep, err := scanEpisode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ep)
	}
	return out, rows.Err()
}

func (s *Store) SetStatus(ctx context.Context, episodeID int64, status string, startedAt, finishedAt *time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE episodes SET status = ?, started_at = ?, finished_at = ? WHERE id = ?`,
		status, startedAt, finishedAt, episodeID)
	if err != nil {
		return fmt.Errorf("setting episode status: %w", err)
	}
	return nil
}

// SetPlaybackProgress records the last known playback position, for
// rendering a per-episode progress bar. Pass durationSecs <= 0 to clear it
// (e.g. once an episode is fully watched).
func (s *Store) SetPlaybackProgress(ctx context.Context, episodeID int64, positionSecs, durationSecs float64) error {
	var posArg, durArg any
	if durationSecs > 0 {
		posArg, durArg = positionSecs, durationSecs
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE episodes SET resume_position_seconds = ?, duration_seconds = ? WHERE id = ?`,
		posArg, durArg, episodeID)
	if err != nil {
		return fmt.Errorf("setting playback progress: %w", err)
	}
	return nil
}

// RenameSeries updates a series' title and dir_path, and rewrites every one
// of its episodes' file_path to sit under the new directory (file names are
// left as-is; only the directory prefix moves). Call this only after the
// directory itself has already been renamed on disk — this just brings the
// database in line with that.
func (s *Store) RenameSeries(ctx context.Context, seriesID int64, newTitle, oldDirPath, newDirPath string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE series SET title = ?, dir_path = ? WHERE id = ?`, newTitle, newDirPath, seriesID); err != nil {
		return fmt.Errorf("renaming series: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `SELECT id, file_path FROM episodes WHERE series_id = ?`, seriesID)
	if err != nil {
		return fmt.Errorf("querying episodes: %w", err)
	}
	type idPath struct {
		id   int64
		path string
	}
	var eps []idPath
	for rows.Next() {
		var ip idPath
		if err := rows.Scan(&ip.id, &ip.path); err != nil {
			rows.Close()
			return fmt.Errorf("scanning episode: %w", err)
		}
		eps = append(eps, ip)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterating episodes: %w", err)
	}
	rows.Close()

	for _, ep := range eps {
		rel, err := filepath.Rel(oldDirPath, ep.path)
		if err != nil {
			return fmt.Errorf("computing relative path for %q: %w", ep.path, err)
		}
		newPath := filepath.Join(newDirPath, rel)
		if _, err := tx.ExecContext(ctx, `UPDATE episodes SET file_path = ? WHERE id = ?`, newPath, ep.id); err != nil {
			return fmt.Errorf("updating episode path: %w", err)
		}
	}

	return tx.Commit()
}

// DeleteSeries removes a series from the database; its episodes go with it
// via the FK cascade. Call this only after the directory has already been
// deleted from disk.
func (s *Store) DeleteSeries(ctx context.Context, seriesID int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM series WHERE id = ?`, seriesID); err != nil {
		return fmt.Errorf("deleting series: %w", err)
	}
	return nil
}

// RenameEpisode updates one episode's file_name/file_path/episode_number.
// Call this only after the file has already been renamed on disk.
func (s *Store) RenameEpisode(ctx context.Context, episodeID int64, newFileName, newFilePath string, epNum *int) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE episodes SET file_name = ?, file_path = ?, episode_number = ? WHERE id = ?`,
		newFileName, newFilePath, epNum, episodeID)
	if err != nil {
		return fmt.Errorf("renaming episode: %w", err)
	}
	return nil
}

// DeleteEpisode removes one episode from the database. Call this only after
// the file has already been deleted from disk.
func (s *Store) DeleteEpisode(ctx context.Context, episodeID int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM episodes WHERE id = ?`, episodeID); err != nil {
		return fmt.Errorf("deleting episode: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEpisode(row rowScanner) (Episode, error) {
	var ep Episode
	var epNum sql.NullInt64
	var modTime sql.NullTime
	var startedAt, finishedAt sql.NullTime
	var resumePos, duration sql.NullFloat64
	err := row.Scan(&ep.ID, &ep.SeriesID, &ep.FilePath, &ep.FileName, &epNum, &ep.SizeBytes, &modTime, &ep.Status, &startedAt, &finishedAt, &resumePos, &duration)
	if err != nil {
		return Episode{}, fmt.Errorf("scanning episode: %w", err)
	}
	if epNum.Valid {
		n := int(epNum.Int64)
		ep.EpisodeNumber = &n
	}
	if modTime.Valid {
		ep.ModTime = modTime.Time
	}
	if startedAt.Valid {
		t := startedAt.Time
		ep.StartedAt = &t
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		ep.FinishedAt = &t
	}
	if resumePos.Valid {
		v := resumePos.Float64
		ep.ResumePositionSecs = &v
	}
	if duration.Valid {
		v := duration.Float64
		ep.DurationSecs = &v
	}
	return ep, nil
}
