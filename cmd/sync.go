package cmd

import (
	"fmt"
	"path"

	"github.com/spf13/cobra"

	"anime-tracker/internal/qbt"
	"anime-tracker/internal/remote"
	"anime-tracker/internal/scanner"
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
		sshTarget, err := requiredSetting(ctx, store, "remote.ssh_target")
		if err != nil {
			return err
		}

		client, err := connectQBT(ctx, store)
		if err != nil {
			return err
		}

		torrents, err := client.ListTorrents(ctx, qbtTag)
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

		if syncDryRun {
			for _, t := range completed {
				fmt.Printf("would sync: %s (%s)\n", t.Name, t.ContentPath)
			}
			fmt.Printf("%d finished, %d still downloading (dry run — nothing changed)\n", len(completed), pending)
			return nil
		}

		var synced, failed []string
		for _, t := range completed {
			seriesName := path.Base(t.SavePath)
			localDir, isNew, err := remote.ResolveLocalSeriesDir(cfg.Dir, seriesName)
			if err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", t.Name, err))
				continue
			}
			if isNew {
				fmt.Printf("new series folder: %s\n", localDir)
			}

			if err := remote.Fetch(ctx, sshTarget, t.ContentPath, localDir); err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", t.Name, err))
				continue
			}
			if err := remote.FlattenDir(localDir); err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", t.Name, err))
				continue
			}
			if err := client.RemoveTags(ctx, []string{t.Hash}, qbtTag); err != nil {
				failed = append(failed, fmt.Sprintf("%s: fetched but failed to un-tag (will re-sync next time): %v", t.Name, err))
				continue
			}
			synced = append(synced, t.Name)
			fmt.Printf("synced: %s\n", t.Name)
		}

		for _, f := range failed {
			fmt.Printf("failed: %s\n", f)
		}

		if len(synced) > 0 {
			res, err := scanner.Scan(ctx, store, cfg.Dir)
			if err != nil {
				return err
			}
			fmt.Printf("scanned %s: %d new series, %d new episodes, %d newly watched\n",
				cfg.Dir, res.NewSeries, res.NewEpisodes, res.NewlyWatched)
		}

		fmt.Printf("%d synced, %d failed, %d still downloading\n", len(synced), len(failed), pending)
		if len(failed) > 0 {
			return fmt.Errorf("%d torrent(s) failed to sync (see above) — still tagged, will retry next time", len(failed))
		}
		return nil
	},
}
