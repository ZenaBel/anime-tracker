package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"anime-tracker/internal/backup"
)

var importYes bool

func init() {
	importCmd.Flags().BoolVarP(&importYes, "yes", "y", false, "skip the confirmation prompt")
}

var exportCmd = &cobra.Command{
	Use:   "export <file.json>",
	Short: "Export watch history (status/progress per episode) to a JSON file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		data, err := backup.Build(cmd.Context(), store)
		if err != nil {
			return err
		}

		f, err := os.Create(args[0])
		if err != nil {
			return fmt.Errorf("creating %s: %w", args[0], err)
		}
		defer f.Close()

		if err := backup.Write(f, data); err != nil {
			return fmt.Errorf("writing %s: %w", args[0], err)
		}
		fmt.Printf("exported %d series to %s\n", len(data.Series), args[0])
		return nil
	},
}

var importCmd = &cobra.Command{
	Use:   "import <file.json>",
	Short: "Import watch history from a JSON file, matched against the already-scanned library",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("opening %s: %w", args[0], err)
		}
		data, err := backup.Read(f)
		f.Close()
		if err != nil {
			return err
		}

		prompt := fmt.Sprintf("Import watch history from %s? This overwrites status/progress for matching episodes.", args[0])
		if !importYes && !confirm(prompt) {
			fmt.Println("cancelled")
			return nil
		}

		store, _, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		res, err := backup.Apply(cmd.Context(), store, data)
		if err != nil {
			return err
		}

		fmt.Printf("matched %d series, updated %d episode(s)\n", res.SeriesMatched, res.EpisodesUpdated)
		if res.EpisodesSkipped > 0 {
			fmt.Printf("skipped %d episode(s) not found in the current library\n", res.EpisodesSkipped)
		}
		for _, title := range res.SeriesSkipped {
			fmt.Printf("series not found locally, skipped: %s\n", title)
		}
		return nil
	},
}
