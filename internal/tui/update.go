package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"anime-tracker/internal/db"
	"anime-tracker/internal/search"
	"anime-tracker/internal/settings"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.searchActive {
			return m.handleSearchKey(msg)
		}
		if m.manage.kind != manageNone {
			return m.handleManageKey(msg)
		}
		if m.settingsActive {
			return m.handleSettingsKey(msg)
		}
		if m.rss.active {
			return m.handleRSSKey(msg)
		}
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

		// A search jump wins over everything else. Otherwise: if the
		// previously selected episode is still in the new list (an
		// in-place refresh of the same series), keep the cursor on it
		// exactly. Otherwise — landing on this series fresh, e.g. from
		// series-pane navigation or on startup — default to the first
		// not-yet-watched episode instead of always episode 1.
		var selectedID int64
		if m.pendingEpisodeID > 0 {
			selectedID = m.pendingEpisodeID
			m.pendingEpisodeID = 0
		} else if ep, ok := m.selectedEpisode(); ok {
			selectedID = ep.ID
		}
		m.episodes = msg.episodes
		m.episodeIdx = indexByID(m.episodes, selectedID, func(e db.Episode) int64 { return e.ID }, db.FirstUnwatchedIndex(m.episodes))
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

	case playlistLaunchedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.statusMsg = fmt.Sprintf("playlist: %d episode(s) queued (tracking via mpv IPC)", len(msg.episodes))
		return m, waitForPlaylistItemCmd(msg.episodes, msg.ch, 0)

	case playlistItemFinishedMsg:
		ep := msg.episodes[msg.result.FileIndex]
		if msg.result.Watched {
			m.statusMsg = ep.FileName + ": watched"
		} else {
			m.statusMsg = ep.FileName + ": stopped partway through"
		}
		return m, tea.Batch(
			playlistItemResultCmd(m.store, ep, msg.result),
			waitForPlaylistItemCmd(msg.episodes, msg.ch, msg.received),
		)

	case playlistDoneMsg:
		if msg.received == 0 {
			m.statusMsg = "mpv's IPC socket never became available — nothing was tracked (mpv is likely still playing normally)"
		} else {
			m.statusMsg = "playlist finished"
		}
		return m, nil

	case searchDataLoadedMsg:
		m.searchLoading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.searchAllEpisodes = msg.episodes
		return m.withSearchResults(), nil

	case manageDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.statusMsg = ""
			return m, nil
		}
		m.err = nil
		m.statusMsg = "done"
		return m, loadSeriesCmd(m.store, m.sortMode)

	case settingsLoadedMsg:
		m.settingsLoading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.settingsValues = msg.values
		return m, nil

	case settingsSavedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		return m, loadSettingsCmd(m.store)

	case rssArticlesLoadedMsg:
		m.rss.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.rss.feeds = groupArticlesByFeed(msg.articles)
		m.rss.feedIdx = 0
		m.rss.articleIdx = 0
		m.rss.focus = rssFocusFeeds
		return m, nil

	case rssDownloadDoneMsg:
		m.rss.submitting = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.statusMsg = "queued: " + m.rss.article.Title
		m.rss = rssState{}
		return m, nil

	case syncDownloadsStartedMsg:
		return m, waitForSyncEventCmd(msg.ch)

	case syncEventMsg:
		ev := msg.event
		if !ev.Done {
			m.statusMsg = fmt.Sprintf("syncing %s: %d%% (%s)", truncate(ev.TorrentName, 40), ev.Progress.Percent, ev.Progress.Rate)
			return m, waitForSyncEventCmd(msg.ch)
		}
		if ev.Err != nil {
			m.err = ev.Err
			m.statusMsg = ""
			return m, nil
		}
		m.err = nil
		res := ev.Result
		m.statusMsg = fmt.Sprintf("sync: %d synced, %d failed, %d still downloading", len(res.Synced), len(res.Failed), res.Pending)
		if len(res.Failed) > 0 {
			m.err = fmt.Errorf("sync-downloads: %s", strings.Join(res.Failed, "; "))
		}
		if res.Scanned {
			return m, loadSeriesCmd(m.store, m.sortMode)
		}
		return m, nil
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

	case "/":
		m.searchActive = true
		m.searchQuery = ""
		m.searchScope = search.ScopeAll
		m.searchIdx = 0
		m.searchLoading = true
		return m.withSearchResults(), loadSearchDataCmd(m.store)

	case "c":
		m.settingsActive = true
		m.settingsLoading = true
		m.settingsEditing = false
		m.settingsIdx = 0
		return m, loadSettingsCmd(m.store)

	case "g":
		m.rss = rssState{active: true, loading: true}
		return m, loadRSSArticlesCmd(m.store)

	case "S":
		m.statusMsg = "syncing downloads..."
		m.err = nil
		return m, syncDownloadsCmd(m.store, m.rootDir)

	case "p":
		if len(m.episodes) == 0 {
			return m, nil
		}
		queue := append([]db.Episode(nil), m.episodes[db.FirstUnwatchedIndex(m.episodes):]...)
		m.statusMsg = "launching playlist..."
		return m, playlistOpenCmd(queue)

	case "R":
		if m.focus == focusSeries {
			if s, ok := m.selectedSeries(); ok {
				m.manage = manageState{kind: manageRenameSeries, input: s.Title}
			}
			return m, nil
		}
		if ep, ok := m.selectedEpisode(); ok {
			m.manage = manageState{kind: manageRenameEpisode, input: ep.FileName}
		}
		return m, nil

	case "D":
		if m.focus == focusSeries {
			if _, ok := m.selectedSeries(); ok {
				m.manage = manageState{kind: manageDeleteSeries}
			}
			return m, nil
		}
		if _, ok := m.selectedEpisode(); ok {
			m.manage = manageState{kind: manageDeleteEpisode}
		}
		return m, nil

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

// handleSearchKey handles input while the search overlay is active: text
// typed builds the query, dedicated keys navigate/act on results, and
// everything re-filters in place (all in-memory, no async round-trip once
// searchAllEpisodes is loaded).
func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.searchActive = false
		return m, nil

	case tea.KeyEnter:
		if r, ok := m.selectedSearchResult(); ok {
			return m.jumpToSearchResult(r)
		}
		m.searchActive = false
		return m, nil

	case tea.KeyTab:
		m.searchScope = search.NextScope(m.searchScope)
		m.searchIdx = 0
		return m.withSearchResults(), nil

	case tea.KeyUp:
		m.searchIdx = clamp(m.searchIdx-1, 0, max(0, len(m.searchResults)-1))
		return m, nil

	case tea.KeyDown:
		m.searchIdx = clamp(m.searchIdx+1, 0, max(0, len(m.searchResults)-1))
		return m, nil

	case tea.KeyBackspace:
		if r := []rune(m.searchQuery); len(r) > 0 {
			m.searchQuery = string(r[:len(r)-1])
			m.searchIdx = 0
			return m.withSearchResults(), nil
		}
		return m, nil

	case tea.KeyRunes:
		m.searchQuery += string(msg.Runes)
		m.searchIdx = 0
		return m.withSearchResults(), nil

	case tea.KeySpace:
		m.searchQuery += " "
		m.searchIdx = 0
		return m.withSearchResults(), nil
	}

	return m, nil
}

// handleManageKey routes to the rename or delete-confirm handler depending
// on which overlay is active.
func (m Model) handleManageKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.manage.kind {
	case manageRenameSeries, manageRenameEpisode:
		return m.handleRenameKey(msg)
	case manageDeleteSeries, manageDeleteEpisode:
		return m.handleDeleteConfirmKey(msg)
	}
	return m, nil
}

func (m Model) handleRenameKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.manage = manageState{}
		return m, nil

	case tea.KeyEnter:
		kind := m.manage.kind
		input := strings.TrimSpace(m.manage.input)
		m.manage = manageState{}
		if input == "" {
			return m, nil
		}
		switch kind {
		case manageRenameSeries:
			s, ok := m.selectedSeries()
			if !ok || input == s.Title {
				return m, nil
			}
			m.statusMsg = "renaming..."
			return m, renameSeriesCmd(m.store, s, input)
		case manageRenameEpisode:
			ep, ok := m.selectedEpisode()
			if !ok || input == ep.FileName {
				return m, nil
			}
			m.statusMsg = "renaming..."
			return m, renameEpisodeCmd(m.store, ep, input)
		}
		return m, nil

	case tea.KeyBackspace:
		if r := []rune(m.manage.input); len(r) > 0 {
			m.manage.input = string(r[:len(r)-1])
		}
		return m, nil

	case tea.KeyRunes:
		m.manage.input += string(msg.Runes)
		return m, nil

	case tea.KeySpace:
		m.manage.input += " "
		return m, nil
	}
	return m, nil
}

// handleDeleteConfirmKey requires an explicit "y"/"Y"/enter to proceed;
// every other key (including esc) cancels.
func (m Model) handleDeleteConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	confirmed := msg.Type == tea.KeyEnter || msg.String() == "y" || msg.String() == "Y"
	kind := m.manage.kind
	m.manage = manageState{}
	if !confirmed {
		return m, nil
	}
	switch kind {
	case manageDeleteSeries:
		if s, ok := m.selectedSeries(); ok {
			m.statusMsg = "deleting..."
			return m, deleteSeriesCmd(m.store, s)
		}
	case manageDeleteEpisode:
		if ep, ok := m.selectedEpisode(); ok {
			m.statusMsg = "deleting..."
			return m, deleteEpisodeCmd(m.store, ep)
		}
	}
	return m, nil
}

// handleSettingsKey routes to the edit-mode handler when a field is
// currently being edited, otherwise handles browsing the settings list.
func (m Model) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.settingsEditing {
		return m.handleSettingsEditKey(msg)
	}

	switch msg.String() {
	case "esc":
		m.settingsActive = false
		return m, nil

	case "up", "k":
		m.settingsIdx = clamp(m.settingsIdx-1, 0, len(settings.Keys)-1)
		return m, nil

	case "down", "j":
		m.settingsIdx = clamp(m.settingsIdx+1, 0, len(settings.Keys)-1)
		return m, nil

	case "enter":
		key := settings.Keys[m.settingsIdx]
		m.settingsEditing = true
		if key == settings.PasswordKey {
			m.settingsInput = "" // never pre-fill/display a stored password
		} else {
			m.settingsInput = m.settingsValues[key]
		}
		return m, nil

	case "x":
		key := settings.Keys[m.settingsIdx]
		return m, unsetSettingCmd(m.store, key)
	}
	return m, nil
}

func (m Model) handleSettingsEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.settingsEditing = false
		return m, nil

	case tea.KeyEnter:
		key := settings.Keys[m.settingsIdx]
		value := strings.TrimSpace(m.settingsInput)
		m.settingsEditing = false
		if value == "" {
			return m, nil // no-op; 'x' clears a value instead
		}
		return m, saveSettingCmd(m.store, key, value)

	case tea.KeyBackspace:
		if r := []rune(m.settingsInput); len(r) > 0 {
			m.settingsInput = string(r[:len(r)-1])
		}
		return m, nil

	case tea.KeyRunes:
		m.settingsInput += string(msg.Runes)
		return m, nil

	case tea.KeySpace:
		m.settingsInput += " "
		return m, nil
	}
	return m, nil
}

// handleRSSKey handles the RSS overlay: a feeds pane on the left (a
// synthetic "Unread" aggregate plus each real subscribed feed) and that
// feed's articles on the right, navigated the same way as the main view's
// series/episodes panes. Enter on an article downloads it immediately —
// save path is always flat (see settings.RemoteDownloadSavePath), so
// there's nothing left to confirm before submitting.
func (m Model) handleRSSKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.rss = rssState{}
		return m, nil

	case "up", "k":
		switch m.rss.focus {
		case rssFocusFeeds:
			if len(m.rss.feeds) > 0 {
				m.rss.feedIdx = clamp(m.rss.feedIdx-1, 0, len(m.rss.feeds)-1)
				m.rss.articleIdx = 0
			}
		case rssFocusArticles:
			if arts := m.rss.currentArticles(); len(arts) > 0 {
				m.rss.articleIdx = clamp(m.rss.articleIdx-1, 0, len(arts)-1)
			}
		}
		return m, nil

	case "down", "j":
		switch m.rss.focus {
		case rssFocusFeeds:
			if len(m.rss.feeds) > 0 {
				m.rss.feedIdx = clamp(m.rss.feedIdx+1, 0, len(m.rss.feeds)-1)
				m.rss.articleIdx = 0
			}
		case rssFocusArticles:
			if arts := m.rss.currentArticles(); len(arts) > 0 {
				m.rss.articleIdx = clamp(m.rss.articleIdx+1, 0, len(arts)-1)
			}
		}
		return m, nil

	case "left", "h":
		m.rss.focus = rssFocusFeeds
		return m, nil

	case "right", "l":
		if m.rss.focus == rssFocusFeeds {
			m.rss.focus = rssFocusArticles
		}
		return m, nil

	case "enter":
		if m.rss.focus == rssFocusFeeds {
			m.rss.focus = rssFocusArticles
			return m, nil
		}
		arts := m.rss.currentArticles()
		if m.rss.articleIdx < 0 || m.rss.articleIdx >= len(arts) {
			return m, nil
		}
		article := arts[m.rss.articleIdx]
		m.rss.article = article
		m.rss.submitting = true
		m.statusMsg = "queuing " + article.Title + "..."
		return m, submitRSSDownloadCmd(m.store, article)
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
