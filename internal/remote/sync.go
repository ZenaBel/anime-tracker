// Package remote pulls finished downloads from a remote host into the
// local library: transferring files over rsync/ssh, then normalizing
// whatever qBittorrent handed back into the flat one-folder-per-series
// layout the rest of anime-tracker expects.
package remote

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// buildRsyncArgs constructs the argv for pulling remotePath (a file or a
// directory) from sshTarget down into localDir. -s/--protect-args stops
// the remote shell from reinterpreting brackets/spaces that are common in
// anime release filenames.
func buildRsyncArgs(sshTarget, remotePath, localDir string) []string {
	return []string{
		"-avz",
		"-s",
		"-e", "ssh",
		sshTarget + ":" + remotePath,
		localDir + string(filepath.Separator),
	}
}

// Fetch rsyncs remotePath (a file or directory) from sshTarget into
// localDir, creating localDir first if needed.
func Fetch(ctx context.Context, sshTarget, remotePath, localDir string) error {
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", localDir, err)
	}
	cmd := exec.CommandContext(ctx, "rsync", buildRsyncArgs(sshTarget, remotePath, localDir)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync failed: %w\n%s", err, out)
	}
	return nil
}

// FlattenDir moves every *.mkv found in a subdirectory of dir up to dir
// itself (a torrent may have arrived as a release-group-named batch
// folder, but the scanner only looks at direct Series/*.mkv children), then
// removes whatever subdirectories are left empty afterward. A name
// collision with a file already at the top level is skipped, not
// overwritten, and reported in the returned error.
func FlattenDir(dir string) error {
	var moveErrs []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || path == dir {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".mkv") {
			return nil
		}
		if filepath.Dir(path) == dir {
			return nil // already at the top level
		}

		dest := filepath.Join(dir, filepath.Base(path))
		if _, err := os.Stat(dest); err == nil {
			moveErrs = append(moveErrs, fmt.Sprintf("%s: a file named %q already exists at the top level", path, filepath.Base(path)))
			return nil
		}
		if err := os.Rename(path, dest); err != nil {
			moveErrs = append(moveErrs, fmt.Sprintf("%s: %v", path, err))
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking %s: %w", dir, err)
	}

	removeEmptySubdirs(dir)

	if len(moveErrs) > 0 {
		return fmt.Errorf("flattening %s: %s", dir, strings.Join(moveErrs, "; "))
	}
	return nil
}

// removeEmptySubdirs removes every now-empty subdirectory under dir,
// deepest first. Best-effort: a non-empty or otherwise unremovable
// directory is silently left in place.
func removeEmptySubdirs(dir string) {
	var subdirs []string
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != dir {
			subdirs = append(subdirs, path)
		}
		return nil
	})
	for i := len(subdirs) - 1; i >= 0; i-- {
		os.Remove(subdirs[i]) // no-op if not empty
	}
}

// ResolveLocalSeriesDir maps a remote save-path's folder name onto a local
// library folder: an existing folder is matched case-insensitively (so
// anime-tracker never creates a second, differently-cased copy of a series
// it already tracks); if none matches, it's treated as a brand-new show
// and given a fresh folder using remoteBasename's exact casing.
func ResolveLocalSeriesDir(libraryRoot, remoteBasename string) (path string, isNew bool, err error) {
	entries, err := os.ReadDir(libraryRoot)
	if err != nil {
		return "", false, fmt.Errorf("reading library root: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() && strings.EqualFold(e.Name(), remoteBasename) {
			return filepath.Join(libraryRoot, e.Name()), false, nil
		}
	}
	return filepath.Join(libraryRoot, remoteBasename), true, nil
}
