package cmd

import (
	"context"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"anime-tracker/internal/db"
)

type appConfig struct {
	Dir    string
	DBPath string
}

type ctxKey struct{}

var (
	dirFlag string
	dbFlag  string
)

var rootCmd = &cobra.Command{
	Use:   "anime-tracker",
	Short: "Track which anime episodes you've watched by scanning your library",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		dir, err := resolveDir()
		if err != nil {
			return err
		}
		dbPath, err := resolveDBPath()
		if err != nil {
			return err
		}
		cmd.SetContext(context.WithValue(cmd.Context(), ctxKey{}, appConfig{Dir: dir, DBPath: dbPath}))
		return nil
	},
	RunE: runTUI,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dirFlag, "dir", "", "root dir with series subfolders (default: $ANIME_TRACKER_DIR or the current directory)")
	rootCmd.PersistentFlags().StringVar(&dbFlag, "db", "", "sqlite db path (default: $ANIME_TRACKER_DB or ~/.config/anime-tracker/anime.db)")

	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(playCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(unwatchCmd)
	rootCmd.AddCommand(tuiCmd)
}

func Execute() error {
	return rootCmd.ExecuteContext(context.Background())
}

func resolveDir() (string, error) {
	if dirFlag != "" {
		return filepath.Abs(dirFlag)
	}
	if v := os.Getenv("ANIME_TRACKER_DIR"); v != "" {
		return filepath.Abs(v)
	}
	return os.Getwd()
}

func resolveDBPath() (string, error) {
	if dbFlag != "" {
		return dbFlag, nil
	}
	if v := os.Getenv("ANIME_TRACKER_DB"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "anime-tracker", "anime.db"), nil
}

func configFromCmd(cmd *cobra.Command) appConfig {
	return cmd.Context().Value(ctxKey{}).(appConfig)
}

func openStore(cmd *cobra.Command) (*db.Store, appConfig, func() error, error) {
	cfg := configFromCmd(cmd)
	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, cfg, nil, err
	}
	return db.NewStore(conn), cfg, conn.Close, nil
}
