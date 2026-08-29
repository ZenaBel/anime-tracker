package library

import (
	"context"
	"os"
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

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fake mkv data"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRenameSeries(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t)
	ctx := context.Background()

	oldDir := filepath.Join(root, "Old Title")
	writeFile(t, filepath.Join(oldDir, "01.mkv"))
	writeFile(t, filepath.Join(oldDir, "02.mkv"))

	seriesID, _, err := store.UpsertSeries(ctx, "Old Title", oldDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"01.mkv", "02.mkv"} {
		if _, err := store.UpsertEpisodeSeen(ctx, seriesID, filepath.Join(oldDir, f), f, nil, 0, time.Now()); err != nil {
			t.Fatal(err)
		}
	}

	all, err := store.ListSeriesWithProgress(ctx, db.SortAlphaAsc)
	if err != nil {
		t.Fatal(err)
	}
	s := all[0]

	if err := RenameSeries(ctx, store, s, "New Title"); err != nil {
		t.Fatalf("RenameSeries() error = %v", err)
	}

	newDir := filepath.Join(root, "New Title")
	if _, err := os.Stat(newDir); err != nil {
		t.Fatalf("renamed dir missing on disk: %v", err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("old dir still present on disk")
	}

	all, err = store.ListSeriesWithProgress(ctx, db.SortAlphaAsc)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Title != "New Title" || all[0].DirPath != newDir {
		t.Fatalf("series row after rename = %+v", all)
	}

	eps, err := store.ListEpisodesBySeries(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 2 {
		t.Fatalf("got %d episodes, want 2", len(eps))
	}
	for _, ep := range eps {
		wantPath := filepath.Join(newDir, ep.FileName)
		if ep.FilePath != wantPath {
			t.Errorf("episode %s file_path = %s, want %s", ep.FileName, ep.FilePath, wantPath)
		}
		if _, err := os.Stat(ep.FilePath); err != nil {
			t.Errorf("episode file missing at new path: %v", err)
		}
	}
}

func TestRenameSeries_TargetExists(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t)
	ctx := context.Background()

	oldDir := filepath.Join(root, "Old")
	writeFile(t, filepath.Join(oldDir, "01.mkv"))
	writeFile(t, filepath.Join(root, "Taken", "01.mkv"))

	seriesID, _, err := store.UpsertSeries(ctx, "Old", oldDir)
	if err != nil {
		t.Fatal(err)
	}
	s := db.SeriesProgress{ID: seriesID, Title: "Old", DirPath: oldDir}

	if err := RenameSeries(ctx, store, s, "Taken"); err == nil {
		t.Fatal("expected error when target directory already exists")
	}
	if _, err := os.Stat(oldDir); err != nil {
		t.Fatalf("original dir should be untouched: %v", err)
	}
}

func TestDeleteSeries(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t)
	ctx := context.Background()

	dir := filepath.Join(root, "Gone Show")
	writeFile(t, filepath.Join(dir, "01.mkv"))
	writeFile(t, filepath.Join(dir, "02.mkv"))

	seriesID, _, err := store.UpsertSeries(ctx, "Gone Show", dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertEpisodeSeen(ctx, seriesID, filepath.Join(dir, "01.mkv"), "01.mkv", nil, 0, time.Now()); err != nil {
		t.Fatal(err)
	}

	all, err := store.ListSeriesWithProgress(ctx, db.SortAlphaAsc)
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteSeries(ctx, store, all[0]); err != nil {
		t.Fatalf("DeleteSeries() error = %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("directory should be gone from disk")
	}
	all, err = store.ListSeriesWithProgress(ctx, db.SortAlphaAsc)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("series row should be gone, got %+v", all)
	}
	eps, err := store.ListAllEpisodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 0 {
		t.Fatalf("episode rows should cascade-delete, got %+v", eps)
	}
}

func TestRenameEpisode(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t)
	ctx := context.Background()

	dir := filepath.Join(root, "Show")
	writeFile(t, filepath.Join(dir, "01.mkv"))

	seriesID, _, err := store.UpsertSeries(ctx, "Show", dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertEpisodeSeen(ctx, seriesID, filepath.Join(dir, "01.mkv"), "01.mkv", nil, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	eps, err := store.ListEpisodesBySeries(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	ep := eps[0]

	if err := RenameEpisode(ctx, store, ep, "Show - 01 [1080p].mkv"); err != nil {
		t.Fatalf("RenameEpisode() error = %v", err)
	}

	newPath := filepath.Join(dir, "Show - 01 [1080p].mkv")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("renamed file missing on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "01.mkv")); !os.IsNotExist(err) {
		t.Fatalf("old file still present on disk")
	}

	eps, err = store.ListEpisodesBySeries(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 1 {
		t.Fatalf("got %d episodes, want 1", len(eps))
	}
	got := eps[0]
	if got.FileName != "Show - 01 [1080p].mkv" || got.FilePath != newPath {
		t.Fatalf("episode after rename = %+v", got)
	}
	if got.EpisodeNumber == nil || *got.EpisodeNumber != 1 {
		t.Fatalf("episode_number after rename = %v, want 1", got.EpisodeNumber)
	}
}

func TestRenameEpisode_ExtensionMismatchRejected(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t)
	ctx := context.Background()

	dir := filepath.Join(root, "Show")
	writeFile(t, filepath.Join(dir, "01.mkv"))

	seriesID, _, err := store.UpsertSeries(ctx, "Show", dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertEpisodeSeen(ctx, seriesID, filepath.Join(dir, "01.mkv"), "01.mkv", nil, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	eps, err := store.ListEpisodesBySeries(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}

	if err := RenameEpisode(ctx, store, eps[0], "renamed.mp4"); err == nil {
		t.Fatal("expected error renaming to a different extension")
	}
	if _, err := os.Stat(filepath.Join(dir, "01.mkv")); err != nil {
		t.Fatalf("original file should be untouched: %v", err)
	}
}

func TestDeleteEpisode(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t)
	ctx := context.Background()

	dir := filepath.Join(root, "Show")
	writeFile(t, filepath.Join(dir, "01.mkv"))

	seriesID, _, err := store.UpsertSeries(ctx, "Show", dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertEpisodeSeen(ctx, seriesID, filepath.Join(dir, "01.mkv"), "01.mkv", nil, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	eps, err := store.ListEpisodesBySeries(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}

	if err := DeleteEpisode(ctx, store, eps[0]); err != nil {
		t.Fatalf("DeleteEpisode() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "01.mkv")); !os.IsNotExist(err) {
		t.Fatalf("file should be gone from disk")
	}
	eps, err = store.ListEpisodesBySeries(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 0 {
		t.Fatalf("episode row should be gone, got %+v", eps)
	}
}
