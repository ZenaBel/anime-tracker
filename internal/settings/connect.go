package settings

import (
	"context"
	"fmt"
	"path"

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

// RemoteDownloadSavePath computes where a torrent for seriesTitle should be
// saved on the remote host, for `download`/`rss-download` to pass as
// AddTorrent's savepath.
//
// Defaults to remote.root itself (flat) — the same layout a qBittorrent
// RSS Auto Downloading rule with no "save to a different directory"
// override already uses, which sync-downloads' resolveSeriesNameForSync
// already resolves correctly via search.GuessSeriesForTitle. Set
// remote.download_subfolder=true to get the old <remote.root>/<Series
// Title> per-series-subfolder layout instead.
//
// Why flat is the default: AniLiberty (and most release groups') torrents
// already wrap their own files in a folder named after the release, which
// for a tracked series is typically the series title itself. With the
// per-series-subfolder layout, that meant a completed torrent's
// content_path ended up two levels deep
// (.../<Series Title>/<Series Title>/episode.mkv) — one layer added by
// savepath, one already inside the torrent. sync-downloads' same-name
// merge (see remote.buildRsyncArgs) only collapses one such layer, not
// two, so the second stayed nested until FlattenDir's recursive walk
// cleaned it up — wastefully, re-fetching the whole episode into that
// stray folder on every subsequent sync-downloads run.
func RemoteDownloadSavePath(ctx context.Context, store *db.Store, seriesTitle string) (string, error) {
	root, err := Required(ctx, store, "remote.root")
	if err != nil {
		return "", err
	}
	subfolder, _, err := store.GetSetting(ctx, "remote.download_subfolder")
	if err != nil {
		return "", err
	}
	if subfolder == "true" {
		return path.Join(root, seriesTitle), nil
	}
	return root, nil
}
