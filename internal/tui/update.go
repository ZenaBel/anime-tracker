package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"anime-tracker/internal/db"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case seriesLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil

		// Re-sorting or reloading can shuffle positions (e.g. the
		// "watched" sort mode reacting to a status change), so track
		// selection by series id rather than trusting the old index.
		var selectedID int64
		if s, ok := m.selectedSeries(); ok {
			selectedID = s.ID
		}
		m.series = msg.series
		m.seriesIdx = indexByID(m.series, selectedID, func(s db.SeriesProgress) int64 { return s.ID }, m.seriesIdx)

		if s, ok := m.selectedSeries(); ok {
			return m, loadEpisodesCmd(m.store, s.ID)
		}
		m.episodes = nil
		return m, nil

	case episodesLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil

		// If the previously selected episode is still in the new list
		// (an in-place refresh of the same series), keep the cursor on
		// it exactly. Otherwise — landing on this series fresh, e.g.
		// from series-pane navigation or on startup — default to the
		// first not-yet-watched episode instead of always episode 1.
		var selectedID int64
		if ep, ok := m.selectedEpisode(); ok {
			selectedID = ep.ID
		}
		m.episodes = msg.episodes
		m.episodeIdx = indexByID(m.episodes, selectedID, func(e db.Episode) int64 { return e.ID }, firstUnwatchedIndex(m.episodes))
		return m, nil

	case scanCompleteMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.statusMsg = formatScanSummary(msg.result)
		return m, loadSeriesCmd(m.store, m.sortMode)

	case actionDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		cmds := []tea.Cmd{loadSeriesCmd(m.store, m.sortMode)}
		if s, ok := m.selectedSeries(); ok {
			cmds = append(cmds, loadEpisodesCmd(m.store, s.ID))
		}
		return m, tea.Batch(cmds...)

	case playerLaunchedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		var cmds []tea.Cmd
		if msg.ep.Status == db.StatusNew {
			cmds = append(cmds, setWatchingCmd(m.store, msg.ep))
		}
		if msg.ch != nil {
			m.statusMsg = "playing " + msg.ep.FileName + " (tracking via mpv IPC)"
			cmds = append(cmds, waitForPlaybackCmd(msg.ep, msg.ch))
		} else {
			m.statusMsg = "playing " + msg.ep.FileName
		}
		return m, tea.Batch(cmds...)

	case playbackFinishedMsg:
		if !msg.result.Watched {
			m.statusMsg = msg.ep.FileName + ": playback ended without reaching EOF — left as watching"
			return m, updateProgressCmd(m.store, msg.ep.ID, msg.result.PositionSecs, msg.result.DurationSecs)
		}
		m.statusMsg = msg.ep.FileName + ": marked watched (reached end of file)"
		return m, markWatchedCmd(m.store, msg.ep)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "r":
		m.statusMsg = "scanning..."
		return m, scanCmd(m.store, m.rootDir)

	case "s":
		m.sortMode = nextSortMode(m.sortMode)
		m.statusMsg = "sort: " + m.sortMode.String()
		return m, loadSeriesCmd(m.store, m.sortMode)

	case "up", "k":
		return m.moveSelection(-1)

	case "down", "j":
		return m.moveSelection(1)

	case "left", "h":
		m.focus = focusSeries
		return m, nil

	case "right", "l":
		if m.focus == focusSeries {
			m.focus = focusEpisodes
		}
		return m, nil

	case "enter":
		if m.focus == focusSeries {
			m.focus = focusEpisodes
			return m, nil
		}
		if ep, ok := m.selectedEpisode(); ok {
			m.statusMsg = "opening " + ep.FileName
			return m, playerOpenCmd(ep)
		}
		return m, nil

	case " ":
		if m.focus == focusEpisodes {
			if ep, ok := m.selectedEpisode(); ok {
				return m, toggleStatusCmd(m.store, ep)
			}
		}
		return m, nil
	}

	return m, nil
}

func (m Model) moveSelection(delta int) (tea.Model, tea.Cmd) {
	switch m.focus {
	case focusSeries:
		if len(m.series) == 0 {
			return m, nil
		}
		m.seriesIdx = clamp(m.seriesIdx+delta, 0, len(m.series)-1)
		m.episodeIdx = 0
		if s, ok := m.selectedSeries(); ok {
			return m, loadEpisodesCmd(m.store, s.ID)
		}
		return m, nil

	case focusEpisodes:
		if len(m.episodes) == 0 {
			return m, nil
		}
		m.episodeIdx = clamp(m.episodeIdx+delta, 0, len(m.episodes)-1)
		return m, nil
	}
	return m, nil
}

// firstUnwatchedIndex returns the index of the first episode that isn't
// fully watched (i.e. the next one to watch), or 0 if there is none or the
// list is empty.
func firstUnwatchedIndex(episodes []db.Episode) int {
	for i, ep := range episodes {
		if ep.Status != db.StatusWatched {
			return i
		}
	}
	return 0
}

// indexByID returns the index of the item whose id (via keyFn) matches id,
// so a selection can survive a reload/re-sort that shuffles positions.
// Falls back to fallback (clamped into bounds) if id isn't found — e.g.
// there was no prior selection, or the item is gone.
func indexByID[T any](items []T, id int64, keyFn func(T) int64, fallback int) int {
	for i, item := range items {
		if keyFn(item) == id {
			return i
		}
	}
	return clamp(fallback, 0, max(0, len(items)-1))
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
