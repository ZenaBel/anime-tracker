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
