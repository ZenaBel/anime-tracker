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
	"path"
	"path/filepath"
	"strings"

	"anime-tracker/internal/db"
	"anime-tracker/internal/qbt"
	"anime-tracker/internal/scanner"
	"anime-tracker/internal/settings"
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

// remapPath rewrites a path qBittorrent reported (containerRoot's point of
// view — e.g. "/downloads/Show/ep01.mkv") to the equivalent real path SSH
// actually sees on the host (hostRoot's point of view). Returns rpath
// unchanged if containerRoot or hostRoot is empty, or rpath isn't actually
// under containerRoot.
func remapPath(rpath, containerRoot, hostRoot string) string {
	if containerRoot == "" || hostRoot == "" {
		return rpath
	}
	if rpath == containerRoot {
		return hostRoot
	}
	if rel, ok := strings.CutPrefix(rpath, containerRoot+"/"); ok {
		return hostRoot + "/" + rel
	}
	return rpath
}

// SyncResult summarizes one SyncDownloads call.
type SyncResult struct {
	Synced     []string // torrent names successfully pulled in
	Failed     []string // "name: error" for each torrent that failed
	NewFolders []string // local series folders created for brand-new shows
	Pending    int      // torrents still downloading, left untouched
	Scanned    bool     // whether a library scan actually ran (only if something synced)
	Scan       scanner.Result
}

// SyncDownloads is the shared CLI/TUI entry point for `sync-downloads`: it
// finds every qbt.Tag-ed torrent that's finished downloading, pulls each
// one from the remote host into the matching local series folder
// (creating it if the show is new), flattens away any nested batch-release
// folder, and removes the tag so it isn't picked up again — a torrent
// whose transfer fails keeps its tag and is left for the next run. Ends
// with a library scan if anything was actually synced.
func SyncDownloads(ctx context.Context, store *db.Store, libraryRoot string) (SyncResult, error) {
	sshTarget, err := settings.Required(ctx, store, "remote.ssh_target")
	if err != nil {
		return SyncResult{}, err
	}
	client, err := settings.Connect(ctx, store)
	if err != nil {
		return SyncResult{}, err
	}

	// qBittorrent reports paths from its own point of view — if it runs in
	// a container with the downloads folder bind-mounted somewhere else
	// than its real host path (e.g. host /home/user/downloads mounted as
	// /downloads in the container), remote.root (qBittorrent's view, also
	// what savePath uses when this tool adds a torrent) and
	// remote.host_root (the real path SSH/rsync actually sees) differ.
	// Both are optional: if either is unset, paths are used as-is, which
	// is correct whenever qBittorrent isn't containerized this way.
	containerRoot, _, err := store.GetSetting(ctx, "remote.root")
	if err != nil {
		return SyncResult{}, err
	}
	hostRoot, _, err := store.GetSetting(ctx, "remote.host_root")
	if err != nil {
		return SyncResult{}, err
	}

	torrents, err := client.ListTorrents(ctx, qbt.Tag)
	if err != nil {
		return SyncResult{}, err
	}

	var res SyncResult
	var completed []qbt.Torrent
	for _, t := range torrents {
		if t.Progress >= 1.0 {
			completed = append(completed, t)
		} else {
			res.Pending++
		}
	}

	for _, t := range completed {
		seriesName := path.Base(t.SavePath) // last path segment is unaffected by the remap either way
		localDir, isNew, err := ResolveLocalSeriesDir(libraryRoot, seriesName)
		if err != nil {
			res.Failed = append(res.Failed, fmt.Sprintf("%s: %v", t.Name, err))
			continue
		}
		if isNew {
			res.NewFolders = append(res.NewFolders, localDir)
		}

		remotePath := remapPath(t.ContentPath, containerRoot, hostRoot)
		if err := Fetch(ctx, sshTarget, remotePath, localDir); err != nil {
			res.Failed = append(res.Failed, fmt.Sprintf("%s: %v", t.Name, err))
			continue
		}
		if err := FlattenDir(localDir); err != nil {
			res.Failed = append(res.Failed, fmt.Sprintf("%s: %v", t.Name, err))
			continue
		}
		if err := client.RemoveTags(ctx, []string{t.Hash}, qbt.Tag); err != nil {
			res.Failed = append(res.Failed, fmt.Sprintf("%s: fetched but failed to un-tag (will re-sync next time): %v", t.Name, err))
			continue
		}
		res.Synced = append(res.Synced, t.Name)
	}

	if len(res.Synced) > 0 {
		scanRes, err := scanner.Scan(ctx, store, libraryRoot)
		if err != nil {
			return res, err
		}
		res.Scan = scanRes
		res.Scanned = true
	}
	return res, nil
}
