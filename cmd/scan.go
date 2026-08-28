package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"anime-tracker/internal/scanner"
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan the library and print what changed",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, cfg, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		res, err := scanner.Scan(cmd.Context(), store, cfg.Dir)
		if err != nil {
			return err
		}

		fmt.Printf("scanned %s: %d new series, %d new episodes, %d newly watched\n",
			cfg.Dir, res.NewSeries, res.NewEpisodes, res.NewlyWatched)
		for _, s := range res.SkippedSeries {
			fmt.Printf("  skipped (unreadable): %s\n", s)
		}
		return nil
	},
}
