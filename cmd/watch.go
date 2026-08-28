package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"anime-tracker/internal/db"
)

var watchCmd = &cobra.Command{
	Use:   "watch <query>",
	Short: "Manually mark an episode as watched",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		ctx := cmd.Context()
		ep, err := findEpisodeByQuery(ctx, store, args[0])
		if err != nil {
			return err
		}

		started := ep.StartedAt
		if started == nil {
			now := time.Now()
			started = &now
		}
		now := time.Now()
		if err := store.SetStatus(ctx, ep.ID, db.StatusWatched, started, &now); err != nil {
			return err
		}

		fmt.Printf("marked watched: %s\n", ep.FileName)
		return nil
	},
}
