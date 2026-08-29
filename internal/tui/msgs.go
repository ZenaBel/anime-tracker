package tui

import (
	"anime-tracker/internal/db"
	"anime-tracker/internal/player"
	"anime-tracker/internal/qbt"
	"anime-tracker/internal/remote"
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
// the next one. received counts results seen so far, including this one.
type playlistItemFinishedMsg struct {
	episodes []db.Episode
	ch       <-chan player.PlaylistResult
	result   player.PlaylistResult
	received int
}

// playlistDoneMsg reports that the playlist's result channel closed —
// mpv exited, whether by reaching the end or an early quit. received==0
// means mpv's IPC socket never connected, so nothing was ever tracked.
type playlistDoneMsg struct {
	received int
}

// searchDataLoadedMsg carries every tracked episode, fetched fresh on
// opening search so it can be fuzzy-matched globally (not just within
// whatever series happens to be selected).
type searchDataLoadedMsg struct {
	episodes []db.Episode
	err      error
}

// manageDoneMsg reports the result of a rename/delete action initiated
// from the manage overlay (see model.go's manageState).
type manageDoneMsg struct {
	err error
}

// settingsLoadedMsg carries every stored qBittorrent/SSH setting, fetched
// on opening the settings overlay and again after each edit.
type settingsLoadedMsg struct {
	values map[string]string
	err    error
}

// settingsSavedMsg reports the result of a set/unset from the settings
// overlay.
type settingsSavedMsg struct {
	err error
}

// rssArticlesLoadedMsg carries every unread RSS article qBittorrent's own
// RSS reader has fetched, loaded on opening the RSS overlay.
type rssArticlesLoadedMsg struct {
	articles []qbt.RSSArticle
	err      error
}

// rssDownloadDoneMsg reports the result of submitting a chosen RSS
// article's torrent to the remote qBittorrent.
type rssDownloadDoneMsg struct {
	err error
}

// syncDownloadsDoneMsg reports the result of pulling finished remote
// downloads into the library (see the "S" key / `sync-downloads`).
type syncDownloadsDoneMsg struct {
	result remote.SyncResult
	err    error
}
