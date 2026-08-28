package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"anime-tracker/internal/db"
)

// Result summarizes what a Scan found.
type Result struct {
	NewSeries     int
	NewEpisodes   int
	NewlyWatched  int64
	SkippedSeries []string
}

// Scan walks rootDir (series subdirectories each containing .mkv files
// directly) and syncs what it finds into store. A series directory that
// becomes unreadable mid-scan is skipped (not diffed) rather than having
// its episodes mass-marked watched.
func Scan(ctx context.Context, store *db.Store, rootDir string) (Result, error) {
	rootEntries, err := os.ReadDir(rootDir)
	if err != nil {
		return Result{}, fmt.Errorf("reading root dir: %w", err)
	}

	var res Result
	for _, e := range rootEntries {
		if !e.IsDir() {
			continue
		}
		seriesDir := filepath.Join(rootDir, e.Name())

		files, err := os.ReadDir(seriesDir)
		if err != nil {
			res.SkippedSeries = append(res.SkippedSeries, seriesDir)
			continue
		}

		var mkvFiles []os.DirEntry
		for _, f := range files {
			if !f.IsDir() && strings.EqualFold(filepath.Ext(f.Name()), ".mkv") {
				mkvFiles = append(mkvFiles, f)
			}
		}

		// A top-level directory with no .mkv files and no prior history
		// isn't a series (e.g. a "torrents" folder full of .torrent files
		// sitting next to the series folders) — don't track it.
		_, alreadyTracked, err := store.SeriesIDByDirPath(ctx, seriesDir)
		if err != nil {
			return res, fmt.Errorf("checking series %q: %w", e.Name(), err)
		}
		if !alreadyTracked && len(mkvFiles) == 0 {
			continue
		}

		seriesID, isNew, err := store.UpsertSeries(ctx, e.Name(), seriesDir)
		if err != nil {
			return res, fmt.Errorf("upserting series %q: %w", e.Name(), err)
		}
		if isNew {
			res.NewSeries++
		}

		var seen []string
		for _, f := range mkvFiles {
			fp := filepath.Join(seriesDir, f.Name())
			info, err := f.Info()
			if err != nil {
				continue
			}
			epNum := ParseEpisodeNumber(f.Name())
			wasNew, err := store.UpsertEpisodeSeen(ctx, seriesID, fp, f.Name(), epNum, info.Size(), info.ModTime())
			if err != nil {
				return res, fmt.Errorf("upserting episode %q: %w", fp, err)
			}
			if wasNew {
				res.NewEpisodes++
			}
			seen = append(seen, fp)
		}

		n, err := store.MarkMissingAsWatched(ctx, seriesID, seen)
		if err != nil {
			return res, fmt.Errorf("marking missing episodes watched for %q: %w", e.Name(), err)
		}
		res.NewlyWatched += n
	}
	return res, nil
}
