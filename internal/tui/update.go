package tui

import (
	tea "github.com/charmbracelet/bubbletea"
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
		m.series = msg.series
		if m.seriesIdx >= len(m.series) {
			m.seriesIdx = max(0, len(m.series)-1)
		}
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
		m.episodes = msg.episodes
		if m.episodeIdx >= len(m.episodes) {
			m.episodeIdx = max(0, len(m.episodes)-1)
		}
		return m, nil

	case scanCompleteMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.statusMsg = formatScanSummary(msg.result)
		return m, loadSeriesCmd(m.store)

	case actionDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		cmds := []tea.Cmd{loadSeriesCmd(m.store)}
		if s, ok := m.selectedSeries(); ok {
			cmds = append(cmds, loadEpisodesCmd(m.store, s.ID))
		}
		return m, tea.Batch(cmds...)
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
			return m, playerOpenCmd(m.store, ep)
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
