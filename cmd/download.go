package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var downloadCmd = &cobra.Command{
	Use:   "download <series-query> <magnet-or-torrent-url>",
	Short: "Send a magnet link/torrent URL to the remote qBittorrent for a tracked series",
	Long: `Resolves <series-query> against your tracked series (fuzzy-matched, same
as play/watch/playlist), then submits <magnet-or-torrent-url> to the remote
qBittorrent configured via 'config set', saved under
<remote.root>/<Series Title> and tagged "anime-tracker" so a later
'sync-downloads' picks it up once it finishes.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		ctx := cmd.Context()
		series, err := findSeriesByQuery(ctx, store, args[0])
		if err != nil {
			return err
		}

		savePath, err := remoteSeriesSavePath(ctx, store, series.Title)
		if err != nil {
			return err
		}

		client, err := connectQBT(ctx, store)
		if err != nil {
			return err
		}
		if err := client.AddTorrent(ctx, args[1], savePath, qbtTag); err != nil {
			return err
		}

		fmt.Printf("queued for %s, saving to %s on the remote host\n", series.Title, savePath)
		return nil
	},
}
