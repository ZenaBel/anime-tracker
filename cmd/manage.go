package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"anime-tracker/internal/db"
	"anime-tracker/internal/library"
	"anime-tracker/internal/search"
)

var manageYes bool

func init() {
	for _, c := range []*cobra.Command{renameSeriesCmd, deleteSeriesCmd, renameEpisodeCmd, deleteEpisodeCmd} {
		c.Flags().BoolVarP(&manageYes, "yes", "y", false, "skip the confirmation prompt")
	}
}

func findSeriesByQuery(ctx context.Context, store *db.Store, query string) (db.SeriesProgress, error) {
	allSeries, err := store.ListSeriesWithProgress(ctx, db.SortAlphaAsc)
	if err != nil {
		return db.SeriesProgress{}, err
	}
	s, ok := search.FindSeries(allSeries, query)
	if !ok {
		return db.SeriesProgress{}, fmt.Errorf("no series matching %q", query)
	}
	return s, nil
}

// confirm prompts on stdout/stdin for a yes/no answer; only an explicit
// "y"/"yes" counts as confirmation.
func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

var renameSeriesCmd = &cobra.Command{
	Use:   "rename-series <query> <new-title>",
	Short: "Rename a series: its on-disk folder and its tracked title",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		ctx := cmd.Context()
		s, err := findSeriesByQuery(ctx, store, args[0])
		if err != nil {
			return err
		}
		newTitle := args[1]
		if !manageYes && !confirm(fmt.Sprintf("Rename %q to %q?", s.Title, newTitle)) {
			fmt.Println("cancelled")
			return nil
		}
		if err := library.RenameSeries(ctx, store, s, newTitle); err != nil {
			return err
		}
		fmt.Printf("renamed: %s -> %s\n", s.Title, newTitle)
		return nil
	},
}

var deleteSeriesCmd = &cobra.Command{
	Use:   "delete-series <query>",
	Short: "Permanently delete a series: its on-disk folder and every episode file in it",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		ctx := cmd.Context()
		s, err := findSeriesByQuery(ctx, store, args[0])
		if err != nil {
			return err
		}
		prompt := fmt.Sprintf("Permanently delete %q and all %d episode file(s)? This cannot be undone.", s.Title, s.Total)
		if !manageYes && !confirm(prompt) {
			fmt.Println("cancelled")
			return nil
		}
		if err := library.DeleteSeries(ctx, store, s); err != nil {
			return err
		}
		fmt.Printf("deleted: %s\n", s.Title)
		return nil
	},
}

var renameEpisodeCmd = &cobra.Command{
	Use:   "rename-episode <query> <new-name>",
	Short: "Rename an episode's file on disk (its extension is kept)",
	Args:  cobra.ExactArgs(2),
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
		newName := args[1]
		if !manageYes && !confirm(fmt.Sprintf("Rename %q to %q?", ep.FileName, newName)) {
			fmt.Println("cancelled")
			return nil
		}
		if err := library.RenameEpisode(ctx, store, ep, newName); err != nil {
			return err
		}
		fmt.Printf("renamed: %s -> %s\n", ep.FileName, newName)
		return nil
	},
}

var deleteEpisodeCmd = &cobra.Command{
	Use:   "delete-episode <query>",
	Short: "Permanently delete one episode's file from disk",
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
		if !manageYes && !confirm(fmt.Sprintf("Permanently delete %q? This cannot be undone.", ep.FileName)) {
			fmt.Println("cancelled")
			return nil
		}
		if err := library.DeleteEpisode(ctx, store, ep); err != nil {
			return err
		}
		fmt.Printf("deleted: %s\n", ep.FileName)
		return nil
	},
}
