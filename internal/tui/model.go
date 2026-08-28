package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"anime-tracker/internal/db"
	"anime-tracker/internal/player"
	"anime-tracker/internal/scanner"
)

type focusPane int

const (
	focusSeries focusPane = iota
	focusEpisodes
)

var sortCycle = []db.SortMode{db.SortAlphaAsc, db.SortAlphaDesc, db.SortAdded, db.SortLastWatched}

type Model struct {
	store   *db.Store
	rootDir string

	series   []db.SeriesProgress
	episodes []db.Episode

	seriesIdx  int
	episodeIdx int
	focus      focusPane
	sortMode   db.SortMode

	width, height int
	statusMsg     string
	err           error
}

func NewModel(store *db.Store, rootDir string) Model {
	return Model{store: store, rootDir: rootDir, sortMode: db.SortAlphaAsc}
}

func (m Model) Init() tea.Cmd {
	return loadSeriesCmd(m.store, m.sortMode)
}

func (m Model) selectedSeries() (db.SeriesProgress, bool) {
	if m.seriesIdx < 0 || m.seriesIdx >= len(m.series) {
		return db.SeriesProgress{}, false
	}
	return m.series[m.seriesIdx], true
}

func (m Model) selectedEpisode() (db.Episode, bool) {
	if m.episodeIdx < 0 || m.episodeIdx >= len(m.episodes) {
		return db.Episode{}, false
	}
	return m.episodes[m.episodeIdx], true
}

func nextSortMode(current db.SortMode) db.SortMode {
	for i, s := range sortCycle {
		if s == current {
			return sortCycle[(i+1)%len(sortCycle)]
		}
	}
	return sortCycle[0]
}

func loadSeriesCmd(store *db.Store, sort db.SortMode) tea.Cmd {
	return func() tea.Msg {
		all, err := store.ListSeriesWithProgress(context.Background(), sort)
		return seriesLoadedMsg{series: all, err: err}
	}
}

func loadEpisodesCmd(store *db.Store, seriesID int64) tea.Cmd {
	return func() tea.Msg {
		eps, err := store.ListEpisodesBySeries(context.Background(), seriesID)
		return episodesLoadedMsg{episodes: eps, err: err}
	}
}

func scanCmd(store *db.Store, rootDir string) tea.Cmd {
	return func() tea.Msg {
		res, err := scanner.Scan(context.Background(), store, rootDir)
		return scanCompleteMsg{result: res, err: err}
	}
}

// playerOpenCmd launches the player and reports back immediately with
// whether an mpv-IPC completion channel is being tracked; it does not
// block on playback finishing.
func playerOpenCmd(ep db.Episode) tea.Cmd {
	return func() tea.Msg {
		ch, err := player.Open(ep.FilePath)
		return playerLaunchedMsg{ep: ep, ch: ch, err: err}
	}
}

func waitForPlaybackCmd(ep db.Episode, ch <-chan player.PlaybackResult) tea.Cmd {
	return func() tea.Msg {
		result := <-ch
		return playbackFinishedMsg{ep: ep, result: result}
	}
}

func setWatchingCmd(store *db.Store, ep db.Episode) tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		err := store.SetStatus(context.Background(), ep.ID, db.StatusWatching, &now, nil)
		return actionDoneMsg{err: err}
	}
}

func markWatched(ctx context.Context, store *db.Store, ep db.Episode) error {
	started := ep.StartedAt
	if started == nil {
		now := time.Now()
		started = &now
	}
	now := time.Now()
	if err := store.SetStatus(ctx, ep.ID, db.StatusWatched, started, &now); err != nil {
		return err
	}
	return store.SetPlaybackProgress(ctx, ep.ID, 0, 0) // clear, episode is done
}

func markWatchedCmd(store *db.Store, ep db.Episode) tea.Cmd {
	return func() tea.Msg {
		return actionDoneMsg{err: markWatched(context.Background(), store, ep)}
	}
}

// updateProgressCmd persists the last known mpv playback position for an
// episode, so a progress bar can be rendered for it.
func updateProgressCmd(store *db.Store, episodeID int64, positionSecs, durationSecs float64) tea.Cmd {
	return func() tea.Msg {
		if durationSecs <= 0 {
			return actionDoneMsg{}
		}
		return actionDoneMsg{err: store.SetPlaybackProgress(context.Background(), episodeID, positionSecs, durationSecs)}
	}
}

func toggleStatusCmd(store *db.Store, ep db.Episode) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if ep.Status == db.StatusWatched {
			err := store.SetStatus(ctx, ep.ID, db.StatusNew, nil, nil)
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{err: markWatched(ctx, store, ep)}
	}
}

// playlistOpenCmd launches episodes as an mpv playlist and reports back
// immediately with the tracking channel; it does not block on the
// playlist finishing.
func playlistOpenCmd(episodes []db.Episode) tea.Cmd {
	return func() tea.Msg {
		paths := make([]string, len(episodes))
		for i, ep := range episodes {
			paths[i] = ep.FilePath
		}
		ch, err := player.OpenPlaylist(paths)
		return playlistLaunchedMsg{episodes: episodes, ch: ch, err: err}
	}
}

func waitForPlaylistItemCmd(episodes []db.Episode, ch <-chan player.PlaylistResult, received int) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return playlistDoneMsg{received: received}
		}
		return playlistItemFinishedMsg{episodes: episodes, ch: ch, result: result, received: received + 1}
	}
}

// playlistItemResultCmd applies one playlist file's outcome to its
// episode: watched episodes are marked done; a still-in-progress one gets
// its status bumped to watching (if it was new) and its position saved.
func playlistItemResultCmd(store *db.Store, ep db.Episode, result player.PlaylistResult) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if result.Watched {
			return actionDoneMsg{err: markWatched(ctx, store, ep)}
		}
		if ep.Status == db.StatusNew {
			now := time.Now()
			if err := store.SetStatus(ctx, ep.ID, db.StatusWatching, &now, nil); err != nil {
				return actionDoneMsg{err: err}
			}
		}
		if result.DurationSecs <= 0 {
			return actionDoneMsg{}
		}
		return actionDoneMsg{err: store.SetPlaybackProgress(ctx, ep.ID, result.PositionSecs, result.DurationSecs)}
	}
}
