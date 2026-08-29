package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"anime-tracker/internal/qbt"
	"anime-tracker/internal/remote"
	"anime-tracker/internal/settings"
)

var syncDryRun bool

func init() {
	syncDownloadsCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "list what would be pulled without touching the network or filesystem")
}

var syncDownloadsCmd = &cobra.Command{
	Use:   "sync-downloads",
	Short: `Pull finished remote downloads (tagged "anime-tracker") into the library and rescan`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, cfg, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		ctx := cmd.Context()

		if syncDryRun {
			client, err := settings.Connect(ctx, store)
			if err != nil {
				return err
			}
			torrents, err := client.ListTorrents(ctx, qbt.Tag)
			if err != nil {
				return err
			}
			var completed []qbt.Torrent
			pending := 0
			for _, t := range torrents {
				if t.Progress >= 1.0 {
					completed = append(completed, t)
				} else {
					pending++
				}
			}
			if len(completed) == 0 {
				fmt.Printf("nothing finished yet (%d still downloading)\n", pending)
				return nil
			}
			for _, t := range completed {
				fmt.Printf("would sync: %s (%s)\n", t.Name, t.ContentPath)
			}
			fmt.Printf("%d finished, %d still downloading (dry run — nothing changed)\n", len(completed), pending)
			return nil
		}

		res, err := remote.SyncDownloads(ctx, store, cfg.Dir)
		if err != nil {
			return err
		}

		for _, f := range res.NewFolders {
			fmt.Printf("new series folder: %s\n", f)
		}
		for _, name := range res.Synced {
			fmt.Printf("synced: %s\n", name)
		}
		for _, f := range res.Failed {
			fmt.Printf("failed: %s\n", f)
		}
		if res.Scanned {
			fmt.Printf("scanned %s: %d new series, %d new episodes, %d newly watched\n",
				cfg.Dir, res.Scan.NewSeries, res.Scan.NewEpisodes, res.Scan.NewlyWatched)
		}

		fmt.Printf("%d synced, %d failed, %d still downloading\n", len(res.Synced), len(res.Failed), res.Pending)
		if len(res.Failed) > 0 {
			return fmt.Errorf("%d torrent(s) failed to sync (see above) — still tagged, will retry next time", len(res.Failed))
		}
		return nil
	},
}
