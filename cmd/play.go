package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"anime-tracker/internal/db"
	"anime-tracker/internal/player"
	"anime-tracker/internal/search"
)

func findEpisodeByQuery(ctx context.Context, store *db.Store, query string) (db.Episode, error) {
	all, err := store.ListAllEpisodes(ctx)
	if err != nil {
		return db.Episode{}, err
	}
	ep, ok := search.FindEpisode(all, query)
	if !ok {
		return db.Episode{}, fmt.Errorf("no episode matching %q", query)
	}
	return ep, nil
}

var playCmd = &cobra.Command{
	Use:   "play <query>",
	Short: "Fuzzy-find an episode and open it in the default player",
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

		ch, err := player.Open(ep.FilePath)
		if err != nil {
			return fmt.Errorf("opening player: %w", err)
		}

		started := ep.StartedAt
		if ep.Status == db.StatusNew {
			now := time.Now()
			started = &now
			if err := store.SetStatus(ctx, ep.ID, db.StatusWatching, started, nil); err != nil {
				return err
			}
		}

		fmt.Printf("playing: %s\n", ep.FileName)

		if ch == nil {
			return nil
		}

		fmt.Println("waiting for mpv to finish (via IPC) to confirm watched...")
		result := <-ch
		if result.DurationSecs > 0 {
			if err := store.SetPlaybackProgress(ctx, ep.ID, result.PositionSecs, result.DurationSecs); err != nil {
				return err
			}
		}
		if !result.Watched {
			fmt.Println("playback ended without reaching the end of the file — left as watching")
			return nil
		}
		finished := time.Now()
		if err := store.SetStatus(ctx, ep.ID, db.StatusWatched, started, &finished); err != nil {
			return err
		}
		if err := store.SetPlaybackProgress(ctx, ep.ID, 0, 0); err != nil {
			return err
		}
		fmt.Println("marked watched (reached end of file)")
		return nil
	},
}
