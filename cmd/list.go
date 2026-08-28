package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"anime-tracker/internal/db"
	"anime-tracker/internal/search"
	"anime-tracker/internal/statusicon"
)

var sortFlag string

func init() {
	listCmd.Flags().StringVar(&sortFlag, "sort", "az", "sort order: az, za, added (newest episode first), watched (recently watched first)")
}

var listCmd = &cobra.Command{
	Use:   "list [series]",
	Short: "List series progress, or episodes of one series",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		sortMode, err := db.ParseSortMode(sortFlag)
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		allSeries, err := store.ListSeriesWithProgress(ctx, sortMode)
		if err != nil {
			return err
		}

		if len(args) == 0 {
			for _, s := range allSeries {
				fmt.Printf("%-40s %3d/%-3d\n", s.Title, s.Watched, s.Total)
			}
			return nil
		}

		series, ok := search.FindSeries(allSeries, args[0])
		if !ok {
			return fmt.Errorf("no series matching %q", args[0])
		}

		episodes, err := store.ListEpisodesBySeries(ctx, series.ID)
		if err != nil {
			return err
		}
		fmt.Printf("%s (%d/%d)\n", series.Title, series.Watched, series.Total)
		for _, ep := range episodes {
			if ep.Status == db.StatusWatching {
				if pct, ok := ep.ProgressPercent(); ok {
					fmt.Printf("  %s %3d%% %s\n", statusicon.Icon(ep.Status), pct, ep.FileName)
					continue
				}
			}
			fmt.Printf("  %s %s\n", statusicon.Icon(ep.Status), ep.FileName)
		}
		return nil
	},
}
