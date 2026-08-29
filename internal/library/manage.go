// Package library implements the destructive on-disk-plus-database
// operations (rename/delete a series or episode) shared by the CLI and the
// TUI. Every function here touches the real filesystem, so each does the
// filesystem side first and only updates the database once that succeeded,
// rolling the filesystem change back if the database write then fails —
// disk and DB should never be left disagreeing about what exists.
package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"anime-tracker/internal/db"
	"anime-tracker/internal/scanner"
)

// RenameSeries renames a series' on-disk directory (newTitle becomes the
// new directory's base name, sitting next to the old one) and updates the
// database — title, dir_path, and every episode's file_path — to match.
func RenameSeries(ctx context.Context, store *db.Store, s db.SeriesProgress, newTitle string) error {
	newTitle = strings.TrimSpace(newTitle)
	if newTitle == "" {
		return fmt.Errorf("new title can't be empty")
	}
	if strings.ContainsAny(newTitle, "/\\") {
		return fmt.Errorf("new title can't contain a path separator")
	}
	if newTitle == s.Title {
		return nil
	}

	newDir := filepath.Join(filepath.Dir(s.DirPath), newTitle)
	if _, err := os.Stat(newDir); err == nil {
		return fmt.Errorf("a folder named %q already exists", newTitle)
	}

	if err := os.Rename(s.DirPath, newDir); err != nil {
		return fmt.Errorf("renaming folder on disk: %w", err)
	}
	if err := store.RenameSeries(ctx, s.ID, newTitle, s.DirPath, newDir); err != nil {
		_ = os.Rename(newDir, s.DirPath) // best-effort: keep disk and DB from diverging
		return err
	}
	return nil
}

// DeleteSeries permanently deletes a series' entire directory — every
// episode file in it included — from disk, then removes it from the
// database (episodes cascade with it).
func DeleteSeries(ctx context.Context, store *db.Store, s db.SeriesProgress) error {
	if err := os.RemoveAll(s.DirPath); err != nil {
		return fmt.Errorf("deleting folder on disk: %w", err)
	}
	return store.DeleteSeries(ctx, s.ID)
}

// RenameEpisode renames one episode's file on disk, keeping it in the same
// series directory and its original extension, and updates the database to
// match.
func RenameEpisode(ctx context.Context, store *db.Store, ep db.Episode, newFileName string) error {
	newFileName = strings.TrimSpace(newFileName)
	if newFileName == "" {
		return fmt.Errorf("new file name can't be empty")
	}
	if strings.ContainsAny(newFileName, "/\\") {
		return fmt.Errorf("new file name can't contain a path separator")
	}

	oldExt := filepath.Ext(ep.FileName)
	switch ext := filepath.Ext(newFileName); {
	case ext == "":
		newFileName += oldExt
	case !strings.EqualFold(ext, oldExt):
		return fmt.Errorf("keep the %s extension (only files with it are tracked on rescan)", oldExt)
	}
	if newFileName == ep.FileName {
		return nil
	}

	newPath := filepath.Join(filepath.Dir(ep.FilePath), newFileName)
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("a file named %q already exists", newFileName)
	}

	if err := os.Rename(ep.FilePath, newPath); err != nil {
		return fmt.Errorf("renaming file on disk: %w", err)
	}
	epNum := scanner.ParseEpisodeNumber(newFileName)
	if err := store.RenameEpisode(ctx, ep.ID, newFileName, newPath, epNum); err != nil {
		_ = os.Rename(newPath, ep.FilePath) // best-effort: keep disk and DB from diverging
		return err
	}
	return nil
}

// DeleteEpisode permanently deletes one episode's file from disk, then
// removes it from the database.
func DeleteEpisode(ctx context.Context, store *db.Store, ep db.Episode) error {
	if err := os.Remove(ep.FilePath); err != nil {
		return fmt.Errorf("deleting file on disk: %w", err)
	}
	return store.DeleteEpisode(ctx, ep.ID)
}
