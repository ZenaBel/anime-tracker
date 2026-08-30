package settings

import (
	"context"
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

// Regression test for a real repro: torrents submitted via
// download/rss-download were saved to <remote.root>/<Series Title>, and
// AniLiberty (and most release groups') torrents already wrap their files
// in a folder named after the release — typically the series title itself
// for a tracked show — so the completed torrent ended up nested two
// levels deep. Flat (remote.root itself) is now the default; the old
// per-series-subfolder behavior is opt-in via remote.download_subfolder.
func TestRemoteDownloadSavePath(t *testing.T) {
	ctx := context.Background()

	t.Run("defaults to flat (remote.root itself)", func(t *testing.T) {
		store := newTestStore(t)
		if err := store.SetSetting(ctx, "remote.root", "/downloads"); err != nil {
			t.Fatal(err)
		}
		got, err := RemoteDownloadSavePath(ctx, store, "Frieren")
		if err != nil {
			t.Fatal(err)
		}
		if got != "/downloads" {
			t.Fatalf("RemoteDownloadSavePath() = %q, want %q", got, "/downloads")
		}
	})

	t.Run("remote.download_subfolder=true opts into the per-series subfolder", func(t *testing.T) {
		store := newTestStore(t)
		if err := store.SetSetting(ctx, "remote.root", "/downloads"); err != nil {
			t.Fatal(err)
		}
		if err := store.SetSetting(ctx, "remote.download_subfolder", "true"); err != nil {
			t.Fatal(err)
		}
		got, err := RemoteDownloadSavePath(ctx, store, "Frieren")
		if err != nil {
			t.Fatal(err)
		}
		if got != "/downloads/Frieren" {
			t.Fatalf("RemoteDownloadSavePath() = %q, want %q", got, "/downloads/Frieren")
		}
	})

	t.Run("any other value for remote.download_subfolder stays flat", func(t *testing.T) {
		store := newTestStore(t)
		if err := store.SetSetting(ctx, "remote.root", "/downloads"); err != nil {
			t.Fatal(err)
		}
		if err := store.SetSetting(ctx, "remote.download_subfolder", "yes"); err != nil {
			t.Fatal(err)
		}
		got, err := RemoteDownloadSavePath(ctx, store, "Frieren")
		if err != nil {
			t.Fatal(err)
		}
		if got != "/downloads" {
			t.Fatalf("RemoteDownloadSavePath() = %q, want %q (only the literal \"true\" opts in)", got, "/downloads")
		}
	})

	t.Run("remote.root unset: clear error", func(t *testing.T) {
		store := newTestStore(t)
		if _, err := RemoteDownloadSavePath(ctx, store, "Frieren"); err == nil {
			t.Fatal("expected an error when remote.root isn't configured")
		}
	})
}
