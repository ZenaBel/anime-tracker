package tui

import (
	"anime-tracker/internal/db"
	"anime-tracker/internal/player"
	"anime-tracker/internal/scanner"
)

type seriesLoadedMsg struct {
	series []db.SeriesProgress
	err    error
}

type episodesLoadedMsg struct {
	episodes []db.Episode
	err      error
}

type scanCompleteMsg struct {
	result scanner.Result
	err    error
}

type actionDoneMsg struct {
	err error
}

// playerLaunchedMsg reports that the player process has been started.
// ch is non-nil only when mpv IPC playback tracking is active.
type playerLaunchedMsg struct {
	ep  db.Episode
	ch  <-chan player.PlaybackResult
	err error
}

// playbackFinishedMsg reports the mpv IPC result once tracked playback
// ends (either at EOF or by early quit).
type playbackFinishedMsg struct {
	ep     db.Episode
	result player.PlaybackResult
}

// playlistLaunchedMsg reports that an mpv playlist has been started. ch is
// nil if launching failed.
type playlistLaunchedMsg struct {
	episodes []db.Episode // in playlist order; result.FileIndex indexes into this
	ch       <-chan player.PlaylistResult
	err      error
}

// playlistItemFinishedMsg reports one file's result within a still-running
// playlist; ch is carried forward so the handler can keep listening for
// the next one.
type playlistItemFinishedMsg struct {
	episodes []db.Episode
	ch       <-chan player.PlaylistResult
	result   player.PlaylistResult
}

// playlistDoneMsg reports that the playlist's result channel closed —
// mpv exited, whether by reaching the end or an early quit.
type playlistDoneMsg struct{}
