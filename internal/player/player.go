package player

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type PlaybackResult struct {
	// Watched is true if mpv reported the file played to its natural end
	// (EOF), false if it was quit/stopped early or tracking failed.
	Watched bool
	// PositionSecs and DurationSecs are the last known playback position
	// and total duration, in seconds, observed over IPC. DurationSecs is
	// 0 if it was never learned (e.g. tracking failed immediately).
	PositionSecs float64
	DurationSecs float64
}

// Open launches filePath in the configured player.
//
// If $ANIME_TRACKER_PLAYER is set to "mpv" (or a path whose base name is
// "mpv"), playback is tracked over mpv's JSON IPC socket and the returned
// channel receives a single PlaybackResult once mpv exits. Otherwise the
// player is launched fire-and-forget (either $ANIME_TRACKER_PLAYER exec'd
// directly, or the OS default opener) and the returned channel is nil.
func Open(filePath string) (<-chan PlaybackResult, error) {
	playerBin := os.Getenv("ANIME_TRACKER_PLAYER")
	if isMPV(playerBin) {
		return openMPVWithIPC(playerBin, filePath)
	}

	cmd, err := build(runtime.GOOS, filePath, playerBin)
	if err != nil {
		return nil, err
	}
	return nil, cmd.Start()
}

// PlaylistResult reports how one file within a playlist launched via
// OpenPlaylist finished.
type PlaylistResult struct {
	FileIndex    int
	Watched      bool
	PositionSecs float64
	DurationSecs float64
}

// OpenPlaylist launches mpv with filePaths queued as a sequential
// playlist. The returned channel receives one PlaylistResult per file, in
// order, as each finishes; it closes once mpv exits, whether that's after
// the last file or from an early quit (any files not yet reached simply
// never get a result). Requires $ANIME_TRACKER_PLAYER=mpv, since playlist
// tracking depends on the same JSON IPC mechanism as single-episode
// tracking.
func OpenPlaylist(filePaths []string) (<-chan PlaylistResult, error) {
	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no files to play")
	}
	playerBin := os.Getenv("ANIME_TRACKER_PLAYER")
	if !isMPV(playerBin) {
		return nil, fmt.Errorf("playlist playback requires ANIME_TRACKER_PLAYER=mpv")
	}
	return openMPVPlaylistWithIPC(playerBin, filePaths)
}

func isMPV(playerBin string) bool {
	return playerBin != "" && filepath.Base(playerBin) == "mpv"
}

func build(goos, filePath, playerBin string) (*exec.Cmd, error) {
	if playerBin != "" {
		return exec.Command(playerBin, filePath), nil
	}
	switch goos {
	case "linux":
		return exec.Command("xdg-open", filePath), nil
	case "darwin":
		return exec.Command("open", filePath), nil
	case "windows":
		return exec.Command("cmd", "/c", "start", "", filePath), nil
	default:
		return nil, fmt.Errorf("unsupported OS: %s", goos)
	}
}
