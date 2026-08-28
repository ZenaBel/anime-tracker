package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"anime-tracker/internal/db"
)

var unwatchCmd = &cobra.Command{
	Use:   "unwatch <query>",
	Short: "Undo a watched mark on an episode",
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

		if err := store.SetStatus(ctx, ep.ID, db.StatusNew, nil, nil); err != nil {
			return err
		}

		fmt.Printf("marked new: %s\n", ep.FileName)
		return nil
	},
}
