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

type Model struct {
	store   *db.Store
	rootDir string

	series   []db.SeriesProgress
	episodes []db.Episode

	seriesIdx  int
	episodeIdx int
	focus      focusPane

	width, height int
	statusMsg     string
	err           error
}

func NewModel(store *db.Store, rootDir string) Model {
	return Model{store: store, rootDir: rootDir}
}

func (m Model) Init() tea.Cmd {
	return loadSeriesCmd(m.store)
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

func loadSeriesCmd(store *db.Store) tea.Cmd {
	return func() tea.Msg {
		all, err := store.ListSeriesWithProgress(context.Background())
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

func playerOpenCmd(store *db.Store, ep db.Episode) tea.Cmd {
	return func() tea.Msg {
		if err := player.Open(ep.FilePath); err != nil {
			return actionDoneMsg{err: err}
		}
		if ep.Status == db.StatusNew {
			now := time.Now()
			err := store.SetStatus(context.Background(), ep.ID, db.StatusWatching, &now, nil)
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{}
	}
}

func toggleStatusCmd(store *db.Store, ep db.Episode) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if ep.Status == db.StatusWatched {
			err := store.SetStatus(ctx, ep.ID, db.StatusNew, nil, nil)
			return actionDoneMsg{err: err}
		}
		started := ep.StartedAt
		if started == nil {
			now := time.Now()
			started = &now
		}
		now := time.Now()
		err := store.SetStatus(ctx, ep.ID, db.StatusWatched, started, &now)
		return actionDoneMsg{err: err}
	}
}
