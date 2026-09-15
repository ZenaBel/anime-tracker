// Package backup implements JSON export/import of watch history, for
// backing up or migrating a library between machines. Absolute paths don't
// survive a move between installs, so matching against the current library
// is done by series folder name and episode file name rather than by the
// full dir_path/file_path stored in the database.
package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"anime-tracker/internal/db"
)

const formatVersion = 1

type Data struct {
	Version    int      `json:"version"`
	ExportedAt time.Time `json:"exported_at"`
	Series     []Series `json:"series"`
}

type Series struct {
	Title    string    `json:"title"`
	DirName  string    `json:"dir_name"`
	Episodes []Episode `json:"episodes"`
}

type Episode struct {
	FileName           string     `json:"file_name"`
	Status             string     `json:"status"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
	ResumePositionSecs *float64   `json:"resume_position_seconds,omitempty"`
	DurationSecs       *float64   `json:"duration_seconds,omitempty"`
}

// Build reads the whole library's watch state out of store.
func Build(ctx context.Context, store *db.Store) (Data, error) {
	allSeries, err := store.ListSeriesWithProgress(ctx, db.SortAlphaAsc)
	if err != nil {
		return Data{}, err
	}

	data := Data{Version: formatVersion, ExportedAt: time.Now()}
	for _, s := range allSeries {
		eps, err := store.ListEpisodesBySeries(ctx, s.ID)
		if err != nil {
			return Data{}, fmt.Errorf("listing episodes for %q: %w", s.Title, err)
		}
		series := Series{Title: s.Title, DirName: filepath.Base(s.DirPath)}
		for _, ep := range eps {
			series.Episodes = append(series.Episodes, Episode{
				FileName:           ep.FileName,
				Status:             ep.Status,
				StartedAt:          ep.StartedAt,
				FinishedAt:         ep.FinishedAt,
				ResumePositionSecs: ep.ResumePositionSecs,
				DurationSecs:       ep.DurationSecs,
			})
		}
		data.Series = append(data.Series, series)
	}
	return data, nil
}

func Write(w io.Writer, data Data) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func Read(r io.Reader) (Data, error) {
	var data Data
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		return Data{}, fmt.Errorf("decoding backup file: %w", err)
	}
	return data, nil
}

// Result summarizes what Apply did, for the caller to report.
type Result struct {
	SeriesMatched   int
	SeriesSkipped   []string // titles not found in the current library
	EpisodesUpdated int
	EpisodesSkipped int // matched series, but episode file not found
}

// Apply overlays data's watch state onto the current library: it never
// creates series or episodes (that's scan's job) — it only updates status
// and progress on rows that already exist, matched by series folder name
// and episode file name rather than absolute path, since those don't
// survive a move between installs.
func Apply(ctx context.Context, store *db.Store, data Data) (Result, error) {
	current, err := store.ListSeriesWithProgress(ctx, db.SortAlphaAsc)
	if err != nil {
		return Result{}, err
	}
	byDirName := make(map[string]db.SeriesProgress, len(current))
	for _, s := range current {
		byDirName[filepath.Base(s.DirPath)] = s
	}

	var res Result
	for _, series := range data.Series {
		cur, ok := byDirName[series.DirName]
		if !ok {
			res.SeriesSkipped = append(res.SeriesSkipped, series.Title)
			continue
		}
		res.SeriesMatched++

		eps, err := store.ListEpisodesBySeries(ctx, cur.ID)
		if err != nil {
			return res, fmt.Errorf("listing episodes for %q: %w", cur.Title, err)
		}
		byFileName := make(map[string]db.Episode, len(eps))
		for _, ep := range eps {
			byFileName[ep.FileName] = ep
		}

		for _, ep := range series.Episodes {
			curEp, ok := byFileName[ep.FileName]
			if !ok {
				res.EpisodesSkipped++
				continue
			}
			if err := store.SetStatus(ctx, curEp.ID, ep.Status, ep.StartedAt, ep.FinishedAt); err != nil {
				return res, fmt.Errorf("updating %q: %w", ep.FileName, err)
			}
			if ep.ResumePositionSecs != nil && ep.DurationSecs != nil {
				if err := store.SetPlaybackProgress(ctx, curEp.ID, *ep.ResumePositionSecs, *ep.DurationSecs); err != nil {
					return res, fmt.Errorf("updating progress for %q: %w", ep.FileName, err)
				}
			}
			res.EpisodesUpdated++
		}
	}
	return res, nil
}
