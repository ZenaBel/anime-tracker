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

// RemoteSeriesSavePath computes where a torrent for seriesTitle should be
// saved on the remote host, following the `<remote.root>/<Series Title>`
// convention `sync-downloads` also expects on the way back.
func RemoteSeriesSavePath(ctx context.Context, store *db.Store, seriesTitle string) (string, error) {
	root, err := Required(ctx, store, "remote.root")
	if err != nil {
		return "", err
	}
	return path.Join(root, seriesTitle), nil
}
