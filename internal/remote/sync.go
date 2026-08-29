// Package remote pulls finished downloads from a remote host into the
// local library: transferring files over rsync/ssh, then normalizing
// whatever qBittorrent handed back into the flat one-folder-per-series
// layout the rest of anime-tracker expects.
package remote

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"anime-tracker/internal/db"
	"anime-tracker/internal/qbt"
	"anime-tracker/internal/scanner"
	"anime-tracker/internal/search"
	"anime-tracker/internal/settings"
)

// buildRsyncArgs constructs the argv for pulling remotePath (a file or a
// directory) from sshTarget down into localDir. -s/--protect-args stops
// the remote shell from reinterpreting brackets/spaces that are common in
// anime release filenames. --info=progress2 emits a periodic aggregate
// progress line for the whole transfer (parsed by parseRsyncProgress);
// --outbuf=L line-buffers it so it arrives as clean newline-terminated
// lines over the pipe instead of the interactive carriage-return-only
// updates rsync uses when it thinks it's talking to a terminal.
//
// If remotePath's own last segment is the same as localDir's (a torrent
// whose content_path is itself a wrapping folder sharing the series'
// name — some releases/qBittorrent do this even for what's logically one
// episode), a trailing slash is added to the source so rsync copies its
// *contents* into localDir directly, instead of nesting that whole
// same-named folder inside itself.
func buildRsyncArgs(sshTarget, remotePath, localDir string) []string {
	src := sshTarget + ":" + remotePath
	if path.Base(remotePath) == filepath.Base(strings.TrimRight(localDir, string(filepath.Separator))) {
		src += "/"
	}
	return []string{
		"-avz",
		"-s",
		"--info=progress2",
		"--outbuf=L",
		"-e", "ssh",
		src,
		localDir + string(filepath.Separator),
	}
}

// FetchProgress is one progress update while a Fetch is running, parsed
// from rsync's own --info=progress2 output.
type FetchProgress struct {
	Percent int
	Rate    string // rsync's own formatted transfer rate, e.g. "12.34MB/s"
}

// progressLineRe matches an --info=progress2 line, e.g.:
//
//	1,234,567  45%   12.34MB/s    0:00:12 (xfr#1, to-chk=0/1)
var progressLineRe = regexp.MustCompile(`^\s*[\d,]+\s+(\d{1,3})%\s+(\S+/s)`)

func parseRsyncProgress(line string) (FetchProgress, bool) {
	m := progressLineRe.FindStringSubmatch(line)
	if m == nil {
		return FetchProgress{}, false
	}
	pct, err := strconv.Atoi(m[1])
	if err != nil {
		return FetchProgress{}, false
	}
	return FetchProgress{Percent: pct, Rate: m[2]}, true
}

// streamRsyncProgress reads r line by line, calling onProgress once per
// distinct percentage reached (throttling out the many identical-percent
// updates rsync emits per second), and returns the last line read (for
// error context if the process then fails). Split out from Fetch so the
// scanning/throttling logic is testable without a real rsync subprocess.
func streamRsyncProgress(r io.Reader, onProgress func(FetchProgress)) (lastLine string) {
	lastPercent := -1
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		lastLine = line
		if p, ok := parseRsyncProgress(line); ok && p.Percent != lastPercent {
			lastPercent = p.Percent
			onProgress(p)
		}
	}
	return lastLine
}

// Fetch rsyncs remotePath (a file or directory) from sshTarget into
// localDir, creating localDir first if needed. If onProgress is non-nil,
// it's called once per distinct percentage reached (not once per raw
// rsync update, which land many times a second) — so a large episode file
// shows real, visible progress instead of the CLI/TUI going silent for
// however long the transfer takes.
func Fetch(ctx context.Context, sshTarget, remotePath, localDir string, onProgress func(FetchProgress)) error {
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", localDir, err)
	}
	cmd := exec.CommandContext(ctx, "rsync", buildRsyncArgs(sshTarget, remotePath, localDir)...)

	if onProgress == nil {
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("rsync failed: %w\n%s", err, out)
		}
		return nil
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("opening rsync stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting rsync: %w", err)
	}

	lastLine := streamRsyncProgress(stdout, onProgress)

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("rsync failed: %w\n%s\n%s", err, lastLine, stderrBuf.String())
	}
	return nil
}

// FlattenDir moves every *.mkv found in a subdirectory of dir up to dir
// itself (a torrent may have arrived as a release-group-named batch
// folder, but the scanner only looks at direct Series/*.mkv children), then
// removes whatever subdirectories are left empty afterward. A name
// collision with a file already at the top level that's the same size is
// treated as the same episode having arrived twice (e.g. a torrent whose
// content wraps a single file in a folder that happens to share the
// series' name — see buildRsyncArgs) and the redundant nested copy is
// just removed; one that's a different size is a genuine conflict, so
// it's left in place untouched and reported in the returned error instead
// of guessing which copy is "right".
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
		if destInfo, err := os.Stat(dest); err == nil {
			srcInfo, srcErr := os.Stat(path)
			if srcErr == nil && srcInfo.Size() == destInfo.Size() {
				if rmErr := os.Remove(path); rmErr != nil {
					moveErrs = append(moveErrs, fmt.Sprintf("%s: duplicate of %s (same size), but couldn't remove it: %v", path, dest, rmErr))
				}
				return nil
			}
			moveErrs = append(moveErrs, fmt.Sprintf("%s: a different file named %q already exists at the top level", path, filepath.Base(path)))
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

// resolveSeriesNameForSync determines which series a completed torrent
// belongs to. Normally that's the last path segment of its save path — the
// per-series subfolder convention download/rss-download and a properly
// configured qBittorrent Auto Downloading rule (with its own "save to a
// different directory") both follow. But if a torrent was saved with no
// subfolder at all — save path equal to containerRoot exactly, e.g. a rule
// left on qBittorrent's plain default save location — that "segment"
// would just be containerRoot's own folder name (e.g. everything piling
// into one shared local "downloads" folder instead of one per series).
// In that degenerate case, the series is guessed from the torrent's own
// name instead (see search.GuessSeriesForTitle), matched only against
// series already tracked locally. ok is false if neither approach can
// determine one.
func resolveSeriesNameForSync(savePath, containerRoot, torrentName string, allSeries []db.SeriesProgress) (string, bool) {
	if containerRoot != "" && strings.TrimRight(savePath, "/") == strings.TrimRight(containerRoot, "/") {
		guess, ok := search.GuessSeriesForTitle(allSeries, torrentName)
		if !ok {
			return "", false
		}
		return guess.Title, true
	}
	return path.Base(savePath), true
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
// with a library scan if anything was actually synced. If onProgress is
// non-nil, it's called with each torrent's name and Fetch progress as its
// transfer runs (see Fetch).
func SyncDownloads(ctx context.Context, store *db.Store, libraryRoot string, onProgress func(torrentName string, p FetchProgress)) (SyncResult, error) {
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

	var allSeries []db.SeriesProgress
	if len(completed) > 0 {
		allSeries, err = store.ListSeriesWithProgress(ctx, db.SortAlphaAsc)
		if err != nil {
			return SyncResult{}, err
		}
	}

	for _, t := range completed {
		seriesName, ok := resolveSeriesNameForSync(t.SavePath, containerRoot, t.Name, allSeries)
		if !ok {
			res.Failed = append(res.Failed, fmt.Sprintf(
				"%s: saved directly to %s with no per-series subfolder, and no tracked series name matches it — set this torrent's/RSS rule's save path to %s/<Series>, or track a matching series first",
				t.Name, containerRoot, containerRoot))
			continue
		}
		localDir, isNew, err := ResolveLocalSeriesDir(libraryRoot, seriesName)
		if err != nil {
			res.Failed = append(res.Failed, fmt.Sprintf("%s: %v", t.Name, err))
			continue
		}
		if isNew {
			res.NewFolders = append(res.NewFolders, localDir)
		}

		remotePath := remapPath(t.ContentPath, containerRoot, hostRoot)
		var progressFn func(FetchProgress)
		if onProgress != nil {
			name := t.Name
			progressFn = func(p FetchProgress) { onProgress(name, p) }
		}
		if err := Fetch(ctx, sshTarget, remotePath, localDir, progressFn); err != nil {
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

// SyncEvent is one update from SyncDownloadsStream: either a progress tick
// (Done == false) or the final result (Done == true, sent exactly once,
// always the last value before the channel closes).
type SyncEvent struct {
	Done        bool
	TorrentName string
	Progress    FetchProgress
	Result      SyncResult
	Err         error
}

// SyncDownloadsStream runs SyncDownloads in the background and streams its
// progress over the returned channel as it happens, for callers (the TUI)
// that need to show live progress rather than block until everything's
// done. The channel receives a SyncEvent per progress tick, then exactly
// one Done event, then closes.
func SyncDownloadsStream(ctx context.Context, store *db.Store, libraryRoot string) <-chan SyncEvent {
	ch := make(chan SyncEvent)
	go func() {
		defer close(ch)
		res, err := SyncDownloads(ctx, store, libraryRoot, func(name string, p FetchProgress) {
			ch <- SyncEvent{TorrentName: name, Progress: p}
		})
		ch <- SyncEvent{Done: true, Result: res, Err: err}
	}()
	return ch
}
