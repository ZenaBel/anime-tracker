package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"anime-tracker/internal/db"
	"anime-tracker/internal/player"
	"anime-tracker/internal/search"
)

var playlistCmd = &cobra.Command{
	Use:   "playlist <series-query>",
	Short: "Play all remaining episodes of a series as one mpv playlist",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		ctx := cmd.Context()
		allSeries, err := store.ListSeriesWithProgress(ctx, db.SortAlphaAsc)
		if err != nil {
			return err
		}
		series, ok := search.FindSeries(allSeries, args[0])
		if !ok {
			return fmt.Errorf("no series matching %q", args[0])
		}

		episodes, err := store.ListEpisodesBySeries(ctx, series.ID)
		if err != nil {
			return err
		}
		if len(episodes) == 0 {
			return fmt.Errorf("%s has no tracked episodes", series.Title)
		}

		queue := episodes[db.FirstUnwatchedIndex(episodes):]
		paths := make([]string, len(queue))
		for i, ep := range queue {
			paths[i] = ep.FilePath
		}

		fmt.Printf("playing %d episode(s) of %s as a playlist...\n", len(queue), series.Title)
		ch, err := player.OpenPlaylist(paths)
		if err != nil {
			return err
		}

		for result := range ch {
			ep := queue[result.FileIndex]

			started := ep.StartedAt
			if ep.Status == db.StatusNew {
				now := time.Now()
				started = &now
				if err := store.SetStatus(ctx, ep.ID, db.StatusWatching, started, nil); err != nil {
					return err
				}
			}

			if err := applyPlaybackResult(ctx, store, ep, started, result.Watched, result.PositionSecs, result.DurationSecs); err != nil {
				return err
			}
			if result.Watched {
				fmt.Printf("  watched: %s\n", ep.FileName)
			} else {
				fmt.Printf("  stopped partway through: %s\n", ep.FileName)
			}
		}

		fmt.Println("playlist finished")
		return nil
	},
}
