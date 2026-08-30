package settings

import (
	"context"
	"fmt"

	"anime-tracker/internal/db"
	"anime-tracker/internal/qbt"
)

// Required fetches key, returning a clear error pointing at how to set it
// (CLI `config set`, or the TUI's 'c' settings overlay) if it isn't
// configured yet.
func Required(ctx context.Context, store *db.Store, key string) (string, error) {
	v, ok, err := store.GetSetting(ctx, key)
	if err != nil {
		return "", err
	}
	if !ok || v == "" {
		return "", fmt.Errorf("%s isn't set — run `anime-tracker config set %s <value>` (or press 'c' in the TUI) first", key, key)
	}
	return v, nil
}

// Connect reads the qbt.* settings and returns a logged-in qBittorrent
// client.
func Connect(ctx context.Context, store *db.Store) (*qbt.Client, error) {
	url, err := Required(ctx, store, "qbt.url")
	if err != nil {
		return nil, err
	}
	username, err := Required(ctx, store, "qbt.username")
	if err != nil {
		return nil, err
	}
	password, err := Required(ctx, store, "qbt.password")
	if err != nil {
		return nil, err
	}
	insecure, _, err := store.GetSetting(ctx, "qbt.insecure_tls")
	if err != nil {
		return nil, err
	}

	c, err := qbt.New(url, insecure == "true")
	if err != nil {
		return nil, err
	}
	if err := c.Login(ctx, username, password); err != nil {
		return nil, err
	}
	return c, nil
}

// RemoteDownloadSavePath returns remote.root as the save path for a
// torrent `download`/`rss-download` submits, to pass as AddTorrent's
// savepath — flat, with no per-series subfolder added.
//
// This used to append <Series Title>, but most release groups' torrents
// already wrap their own files in a folder named after the release, which
// for a tracked show is typically the series title itself. Combined, a
// completed torrent's content_path ended up two folders deep
// (.../<Series Title>/<Series Title>/episode.mkv) — one layer added by
// savepath, one already inside the torrent. sync-downloads' same-name
// merge (see remote.buildRsyncArgs) only collapses one such layer, not
// two, so the second stayed nested until FlattenDir's recursive walk
// cleaned it up — wastefully, re-fetching the whole episode into that
// stray folder on every subsequent sync-downloads run. Flat matches what
// a qBittorrent RSS Auto Downloading rule with no per-rule save-path
// override already uses, which sync-downloads' resolveSeriesNameForSync
// already resolves correctly via search.GuessSeriesForTitle.
func RemoteDownloadSavePath(ctx context.Context, store *db.Store) (string, error) {
	return Required(ctx, store, "remote.root")
}
