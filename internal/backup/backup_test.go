package backup

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"anime-tracker/internal/db"
)

func newTestStore(t *testing.T) *db.Store {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return db.NewStore(conn)
}

func seedSeries(t *testing.T, store *db.Store, dirPath string, episodes []string) int64 {
	t.Helper()
	ctx := context.Background()
	seriesID, _, err := store.UpsertSeries(ctx, filepath.Base(dirPath), dirPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range episodes {
		if _, err := store.UpsertEpisodeSeen(ctx, seriesID, filepath.Join(dirPath, f), f, nil, 0, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	return seriesID
}

// TestRoundTripAcrossDifferentRoot verifies that a backup taken under one
// library root restores cleanly onto a second store where the same series
// live under a completely different root path — the scenario a migration
// between machines needs, since dir_path itself never matches.
func TestRoundTripAcrossDifferentRoot(t *testing.T) {
	ctx := context.Background()

	srcStore := newTestStore(t)
	seriesID := seedSeries(t, srcStore, "/old/root/Show A", []string{"01.mkv", "02.mkv"})

	eps, err := srcStore.ListEpisodesBySeries(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := srcStore.SetStatus(ctx, eps[0].ID, db.StatusWatched, &now, &now); err != nil {
		t.Fatal(err)
	}
	if err := srcStore.SetPlaybackProgress(ctx, eps[1].ID, 300, 1200); err != nil {
		t.Fatal(err)
	}

	data, err := Build(ctx, srcStore)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var buf bytes.Buffer
	if err := Write(&buf, data); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	roundTripped, err := Read(&buf)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	dstStore := newTestStore(t)
	seedSeries(t, dstStore, "/new/root/Show A", []string{"01.mkv", "02.mkv"})

	res, err := Apply(ctx, dstStore, roundTripped)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if res.SeriesMatched != 1 || res.EpisodesUpdated != 2 || res.EpisodesSkipped != 0 || len(res.SeriesSkipped) != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}

	all, err := dstStore.ListSeriesWithProgress(ctx, db.SortAlphaAsc)
	if err != nil {
		t.Fatal(err)
	}
	dstEps, err := dstStore.ListEpisodesBySeries(ctx, all[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if dstEps[0].Status != db.StatusWatched {
		t.Errorf("episode 01 status = %q, want watched", dstEps[0].Status)
	}
	pct, ok := dstEps[1].ProgressPercent()
	if !ok || pct != 25 {
		t.Errorf("episode 02 progress = %d%%, ok=%v, want 25%%", pct, ok)
	}
}

func TestApplySkipsUnmatchedSeriesAndEpisodes(t *testing.T) {
	ctx := context.Background()

	data := Data{
		Version: formatVersion,
		Series: []Series{
			{
				Title:   "Unknown Show",
				DirName: "Unknown Show",
				Episodes: []Episode{
					{FileName: "01.mkv", Status: db.StatusWatched},
				},
			},
			{
				Title:   "Show A",
				DirName: "Show A",
				Episodes: []Episode{
					{FileName: "01.mkv", Status: db.StatusWatched},
					{FileName: "99.mkv", Status: db.StatusWatched},
				},
			},
		},
	}

	store := newTestStore(t)
	seedSeries(t, store, "/root/Show A", []string{"01.mkv"})

	res, err := Apply(ctx, store, data)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if res.SeriesMatched != 1 || res.EpisodesUpdated != 1 || res.EpisodesSkipped != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(res.SeriesSkipped) != 1 || res.SeriesSkipped[0] != "Unknown Show" {
		t.Fatalf("SeriesSkipped = %v, want [Unknown Show]", res.SeriesSkipped)
	}
}
