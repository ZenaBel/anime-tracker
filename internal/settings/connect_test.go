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
// download/rss-download used to be saved to <remote.root>/<Series Title>,
// and most release groups' torrents already wrap their files in a folder
// named after the release — typically the series title itself for a
// tracked show — so the completed torrent ended up nested two levels
// deep. Save path is now always remote.root itself (flat).
func TestRemoteDownloadSavePath(t *testing.T) {
	ctx := context.Background()

	t.Run("returns remote.root as-is", func(t *testing.T) {
		store := newTestStore(t)
		if err := store.SetSetting(ctx, "remote.root", "/downloads"); err != nil {
			t.Fatal(err)
		}
		got, err := RemoteDownloadSavePath(ctx, store)
		if err != nil {
			t.Fatal(err)
		}
		if got != "/downloads" {
			t.Fatalf("RemoteDownloadSavePath() = %q, want %q", got, "/downloads")
		}
	})

	t.Run("remote.root unset: clear error", func(t *testing.T) {
		store := newTestStore(t)
		if _, err := RemoteDownloadSavePath(ctx, store); err == nil {
			t.Fatal("expected an error when remote.root isn't configured")
		}
	})
}
