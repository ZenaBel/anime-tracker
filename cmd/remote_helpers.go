package cmd

import (
	"context"
	"path"

	"anime-tracker/internal/db"
	"anime-tracker/internal/qbt"
)

// qbtTag marks every torrent anime-tracker itself cares about — both ones
// it added directly and ones the user's own qBittorrent RSS Auto
// Downloading rules tag the same way — so `sync-downloads` can find them
// regardless of how they were added.
const qbtTag = "anime-tracker"

// connectQBT reads the qbt.* settings and returns a logged-in client.
func connectQBT(ctx context.Context, store *db.Store) (*qbt.Client, error) {
	url, err := requiredSetting(ctx, store, "qbt.url")
	if err != nil {
		return nil, err
	}
	username, err := requiredSetting(ctx, store, "qbt.username")
	if err != nil {
		return nil, err
	}
	password, err := requiredSetting(ctx, store, "qbt.password")
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

// remoteSeriesSavePath computes where a torrent for series should be saved
// on the remote host, following the `<remote.root>/<Series Title>`
// convention `sync-downloads` also expects on the way back.
func remoteSeriesSavePath(ctx context.Context, store *db.Store, seriesTitle string) (string, error) {
	root, err := requiredSetting(ctx, store, "remote.root")
	if err != nil {
		return "", err
	}
	return path.Join(root, seriesTitle), nil
}
