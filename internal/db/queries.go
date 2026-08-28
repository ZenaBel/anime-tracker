package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	StatusNew      = "new"
	StatusWatching = "watching"
	StatusWatched  = "watched"
)

// Episode is one tracked .mkv file.
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

// SeriesProgress is a series with its aggregated watch progress.
type SeriesProgress struct {
	ID      int64
	Title   string
	DirPath string
	Total   int
	Watched int
}

// SortMode controls the ordering of ListSeriesWithProgress.
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

// ParseSortMode maps a CLI/config sort name to a SortMode.
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

// Store wraps a *sql.DB with the application's queries.
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
	// id is a strictly increasing AUTOINCREMENT key, so it's a reliable
	// insertion-order proxy even when many series are added within the
	// same created_at timestamp tick during one scan.
	SortAdded:       "ORDER BY series.id DESC",
	SortLastWatched: "ORDER BY MAX(episodes.finished_at) IS NULL ASC, MAX(episodes.finished_at) DESC",
}

// ListSeriesWithProgress returns all series with their episode counts,
// ordered per sort.
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

// ListAllEpisodes returns every tracked episode, across all series.
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

// SetStatus manually sets an episode's status and timestamps.
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
