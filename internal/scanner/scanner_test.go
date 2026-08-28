package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
	if err := os.WriteFile(path, []byte("fake"), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func TestScan_NewSeriesAndEpisodes(t *testing.T) {
	root := t.TempDir()
	seriesDir := filepath.Join(root, "Frieren")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(seriesDir, "[Erai-raws] Frieren - 01 (1080p).mkv"))
	writeFile(t, filepath.Join(seriesDir, "[Erai-raws] Frieren - 02 (1080p).mkv"))

	store := newTestStore(t)
	ctx := context.Background()

	res, err := Scan(ctx, store, root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.NewSeries != 1 {
		t.Errorf("NewSeries = %d, want 1", res.NewSeries)
	}
	if res.NewEpisodes != 2 {
		t.Errorf("NewEpisodes = %d, want 2", res.NewEpisodes)
	}
	if res.NewlyWatched != 0 {
		t.Errorf("NewlyWatched = %d, want 0", res.NewlyWatched)
	}

	all, err := store.ListSeriesWithProgress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Total != 2 || all[0].Watched != 0 {
		t.Fatalf("unexpected series progress: %+v", all)
	}
}

func TestScan_RescanIsIdempotent(t *testing.T) {
	root := t.TempDir()
	seriesDir := filepath.Join(root, "Frieren")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(seriesDir, "Frieren - 01.mkv"))

	store := newTestStore(t)
	ctx := context.Background()

	if _, err := Scan(ctx, store, root); err != nil {
		t.Fatal(err)
	}
	res, err := Scan(ctx, store, root)
	if err != nil {
		t.Fatal(err)
	}
	if res.NewSeries != 0 || res.NewEpisodes != 0 {
		t.Errorf("second scan should find nothing new, got %+v", res)
	}
}

func TestScan_DeletedFileBecomesWatched(t *testing.T) {
	root := t.TempDir()
	seriesDir := filepath.Join(root, "Frieren")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	epPath := filepath.Join(seriesDir, "Frieren - 01.mkv")
	writeFile(t, epPath)

	store := newTestStore(t)
	ctx := context.Background()

	if _, err := Scan(ctx, store, root); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(epPath); err != nil {
		t.Fatal(err)
	}

	res, err := Scan(ctx, store, root)
	if err != nil {
		t.Fatal(err)
	}
	if res.NewlyWatched != 1 {
		t.Fatalf("NewlyWatched = %d, want 1", res.NewlyWatched)
	}

	all, err := store.ListSeriesWithProgress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if all[0].Watched != 1 {
		t.Fatalf("Watched = %d, want 1", all[0].Watched)
	}
}

func TestScan_NonSeriesFolderIsNotTracked(t *testing.T) {
	root := t.TempDir()

	seriesDir := filepath.Join(root, "Frieren")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(seriesDir, "Frieren - 01.mkv"))

	// A sibling folder with no .mkv files at all, e.g. a torrent-client
	// download folder full of .torrent files sitting next to the series
	// folders. It must never become a tracked "series".
	torrentsDir := filepath.Join(root, "torrents")
	if err := os.MkdirAll(torrentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(torrentsDir, "Some Show.torrent"))

	store := newTestStore(t)
	ctx := context.Background()

	res, err := Scan(ctx, store, root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.NewSeries != 1 {
		t.Errorf("NewSeries = %d, want 1 (torrents folder must not count)", res.NewSeries)
	}

	all, err := store.ListSeriesWithProgress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Title != "Frieren" {
		t.Fatalf("tracked series = %+v, want only Frieren", all)
	}
}

func TestScan_UnreadableSeriesDirIsSkippedNotMassWatched(t *testing.T) {
	root := t.TempDir()
	seriesDir := filepath.Join(root, "Frieren")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(seriesDir, "Frieren - 01.mkv"))
	writeFile(t, filepath.Join(seriesDir, "Frieren - 02.mkv"))

	store := newTestStore(t)
	ctx := context.Background()

	if _, err := Scan(ctx, store, root); err != nil {
		t.Fatal(err)
	}

	// Simulate the series dir becoming unreadable (e.g. unmounted) rather
	// than genuinely emptied: remove read permission instead of deleting
	// its contents.
	if err := os.Chmod(seriesDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(seriesDir, 0o755) })

	// Running as root bypasses permission checks, which would make this
	// test meaningless — skip in that case.
	if os.Geteuid() == 0 {
		t.Skip("running as root, permission bits don't apply")
	}

	res, err := Scan(ctx, store, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.SkippedSeries) != 1 {
		t.Fatalf("SkippedSeries = %v, want 1 entry", res.SkippedSeries)
	}
	if res.NewlyWatched != 0 {
		t.Fatalf("NewlyWatched = %d, want 0 (series should be skipped, not diffed)", res.NewlyWatched)
	}

	os.Chmod(seriesDir, 0o755)
	all, err := store.ListSeriesWithProgress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if all[0].Watched != 0 {
		t.Fatalf("Watched = %d, want 0 (must not mass-mark watched)", all[0].Watched)
	}
}
