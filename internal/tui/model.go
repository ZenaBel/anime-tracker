package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"anime-tracker/internal/db"
	"anime-tracker/internal/player"
	"anime-tracker/internal/scanner"
	"anime-tracker/internal/search"
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

	searchActive      bool
	searchLoading     bool
	searchQuery       string
	searchScope       search.Scope
	searchAllEpisodes []db.Episode
	searchResults     []search.Result
	searchIdx         int

	// pendingEpisodeID, when >0, is the episode a search jump wants
	// selected once its series' episode list finishes loading; consumed
	// (and cleared) by the next episodesLoadedMsg.
	pendingEpisodeID int64
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

func (m Model) selectedSearchResult() (search.Result, bool) {
	if m.searchIdx < 0 || m.searchIdx >= len(m.searchResults) {
		return search.Result{}, false
	}
	return m.searchResults[m.searchIdx], true
}

// withSearchResults re-runs the fuzzy search against the current query and
// scope, clamping the selection into the new result count.
func (m Model) withSearchResults() Model {
	m.searchResults = search.Search(m.series, m.searchAllEpisodes, m.searchQuery, m.searchScope)
	if m.searchIdx >= len(m.searchResults) {
		m.searchIdx = max(0, len(m.searchResults)-1)
	}
	return m
}

// jumpToSearchResult closes search and selects r in the normal panes: a
// series result focuses the series pane; an episode result focuses the
// episode pane and selects that exact episode once its series' episode
// list (re)loads.
func (m Model) jumpToSearchResult(r search.Result) (Model, tea.Cmd) {
	m.searchActive = false
	m.seriesIdx = indexByID(m.series, r.Series.ID, func(s db.SeriesProgress) int64 { return s.ID }, m.seriesIdx)

	if r.Kind == search.KindSeries {
		m.focus = focusSeries
		m.episodeIdx = 0
		return m, loadEpisodesCmd(m.store, r.Series.ID)
	}
	m.focus = focusEpisodes
	m.pendingEpisodeID = r.Episode.ID
	return m, loadEpisodesCmd(m.store, r.Series.ID)
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

func loadSearchDataCmd(store *db.Store) tea.Cmd {
	return func() tea.Msg {
		eps, err := store.ListAllEpisodes(context.Background())
		return searchDataLoadedMsg{episodes: eps, err: err}
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
