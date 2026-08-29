package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"anime-tracker/internal/db"
	"anime-tracker/internal/scanner"
	"anime-tracker/internal/search"
	"anime-tracker/internal/statusicon"
)

const (
	leftPaneWidth   = 46
	rightPaneWidth  = 60
	searchPaneWidth = leftPaneWidth + rightPaneWidth
	barWidth        = 10
	epBarWidth      = 6
)

var (
	selectedStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	focusedTitle    = lipgloss.NewStyle().Bold(true).Underline(true)
	dimTitle        = lipgloss.NewStyle().Faint(true)
	errStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	helpStyle       = lipgloss.NewStyle().Faint(true)
	leftPaneStyle   = lipgloss.NewStyle().Width(leftPaneWidth).Padding(0, 1)
	rightPaneStyle  = lipgloss.NewStyle().Width(rightPaneWidth).Padding(0, 1)
	searchPaneStyle = lipgloss.NewStyle().Width(searchPaneWidth).Padding(0, 1)
)

// truncate shortens s to at most n runes, appending an ellipsis if cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// nonListRows is how many lines the layout spends on things that aren't
// list rows: pane title + blank line (2), the blank separator and up to
// two footer lines (help + status/error) below the panes (3), and up to
// two scroll indicator lines ("↑ N more" / "↓ N more") within a pane (2).
const nonListRows = 7

// visibleRows returns how many list rows fit in the current terminal
// height. Falls back to a sane default before the first WindowSizeMsg.
func (m Model) visibleRows() int {
	if m.height <= 0 {
		return 20
	}
	n := m.height - nonListRows
	if n < 3 {
		n = 3
	}
	return n
}

// visibleWindow returns the [start, end) slice bounds that keep selected
// within a window of at most maxVisible items out of total.
func visibleWindow(total, selected, maxVisible int) (start, end int) {
	if total <= maxVisible {
		return 0, total
	}
	start = selected - maxVisible/2
	if start < 0 {
		start = 0
	}
	end = start + maxVisible
	if end > total {
		end = total
		start = end - maxVisible
	}
	return start, end
}

func (m Model) View() string {
	if m.searchActive {
		return m.viewSearch()
	}
	if m.manage.kind != manageNone {
		return m.viewManage()
	}

	left := m.viewSeriesPane()
	right := m.viewEpisodesPane()
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	var footer strings.Builder
	footer.WriteString(helpStyle.Render("↑/↓ or j/k: move  ←/→ or h/l: switch pane  enter: open/focus  space: toggle watched  p: playlist  R: rename  D: delete  r: rescan  s: sort (" + m.sortMode.String() + ")  /: search  q: quit"))
	if m.err != nil {
		footer.WriteString("\n")
		footer.WriteString(errStyle.Render("error: " + m.err.Error()))
	} else if m.statusMsg != "" {
		footer.WriteString("\n")
		footer.WriteString(m.statusMsg)
	}

	return body + "\n\n" + footer.String()
}

func (m Model) viewSeriesPane() string {
	title := "Series"
	if m.focus == focusSeries {
		title = focusedTitle.Render(title)
	} else {
		title = dimTitle.Render(title)
	}

	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")

	if len(m.series) == 0 {
		b.WriteString(dimTitle.Render("(no series found — press r to scan)"))
	}

	maxVisible := m.visibleRows()
	start, end := visibleWindow(len(m.series), m.seriesIdx, maxVisible)
	if start > 0 {
		b.WriteString(dimTitle.Render(fmt.Sprintf("  ↑ %d more", start)))
		b.WriteString("\n")
	}
	for i := start; i < end; i++ {
		s := m.series[i]
		line := fmt.Sprintf("%s %-18s %3d/%-3d", progressBar(s.Watched, s.Total), truncate(s.Title, 18), s.Watched, s.Total)
		if i == m.seriesIdx {
			line = selectedStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if end < len(m.series) {
		b.WriteString(dimTitle.Render(fmt.Sprintf("  ↓ %d more", len(m.series)-end)))
		b.WriteString("\n")
	}

	return leftPaneStyle.Render(b.String())
}

func (m Model) viewEpisodesPane() string {
	title := "Episodes"
	if m.focus == focusEpisodes {
		title = focusedTitle.Render(title)
	} else {
		title = dimTitle.Render(title)
	}

	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")

	if len(m.episodes) == 0 {
		b.WriteString(dimTitle.Render("(no episodes)"))
	}

	maxVisible := m.visibleRows()
	start, end := visibleWindow(len(m.episodes), m.episodeIdx, maxVisible)
	if start > 0 {
		b.WriteString(dimTitle.Render(fmt.Sprintf("  ↑ %d more", start)))
		b.WriteString("\n")
	}
	for i := start; i < end; i++ {
		ep := m.episodes[i]
		extra := ""
		if ep.Status == db.StatusWatching {
			if pct, ok := ep.ProgressPercent(); ok {
				extra = fmt.Sprintf(" %s %3d%%", percentBar(pct, epBarWidth), pct)
			}
		}
		nameWidth := rightPaneWidth - 6 - len(extra)
		if nameWidth < 10 {
			nameWidth = 10
		}
		line := fmt.Sprintf("%s%s %s", statusicon.Icon(ep.Status), extra, truncate(ep.FileName, nameWidth))
		if i == m.episodeIdx {
			line = selectedStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if end < len(m.episodes) {
		b.WriteString(dimTitle.Render(fmt.Sprintf("  ↓ %d more", len(m.episodes)-end)))
		b.WriteString("\n")
	}

	return rightPaneStyle.Render(b.String())
}

func (m Model) viewSearch() string {
	var b strings.Builder
	b.WriteString(focusedTitle.Render("Search"))
	b.WriteString(dimTitle.Render(fmt.Sprintf(" (scope: %s — tab to change)", m.searchScope.String())))
	b.WriteString("\n\n")
	b.WriteString("> " + m.searchQuery + "▌")
	b.WriteString("\n\n")

	if m.searchLoading {
		b.WriteString(dimTitle.Render("loading episodes for search..."))
		b.WriteString("\n")
	}

	if len(m.searchResults) == 0 {
		b.WriteString(dimTitle.Render("(no matches)"))
		b.WriteString("\n")
	}

	maxVisible := m.visibleRows()
	start, end := visibleWindow(len(m.searchResults), m.searchIdx, maxVisible)
	if start > 0 {
		b.WriteString(dimTitle.Render(fmt.Sprintf("  ↑ %d more", start)))
		b.WriteString("\n")
	}
	for i := start; i < end; i++ {
		line := formatSearchResult(m.searchResults[i])
		if i == m.searchIdx {
			line = selectedStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if end < len(m.searchResults) {
		b.WriteString(dimTitle.Render(fmt.Sprintf("  ↓ %d more", len(m.searchResults)-end)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("type to filter  tab: scope  ↑/↓: move  enter: jump  esc: cancel"))

	return searchPaneStyle.Render(b.String())
}

func (m Model) viewManage() string {
	var b strings.Builder

	switch m.manage.kind {
	case manageRenameSeries:
		s, _ := m.selectedSeries()
		b.WriteString(focusedTitle.Render("Rename series"))
		b.WriteString("\n\n")
		b.WriteString(dimTitle.Render("current: " + s.Title))
		b.WriteString("\n> " + m.manage.input + "▌\n\n")
		b.WriteString(helpStyle.Render("renames the folder on disk too  ·  enter: confirm  esc: cancel"))

	case manageRenameEpisode:
		ep, _ := m.selectedEpisode()
		b.WriteString(focusedTitle.Render("Rename episode"))
		b.WriteString("\n\n")
		b.WriteString(dimTitle.Render("current: " + ep.FileName))
		b.WriteString("\n> " + m.manage.input + "▌\n\n")
		b.WriteString(helpStyle.Render("renames the file on disk too  ·  enter: confirm  esc: cancel"))

	case manageDeleteSeries:
		s, _ := m.selectedSeries()
		b.WriteString(errStyle.Render("Delete series"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("Permanently delete %q and all %d episode file(s) from disk?\nThis cannot be undone.", s.Title, s.Total))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("y / enter: confirm  ·  any other key: cancel"))

	case manageDeleteEpisode:
		ep, _ := m.selectedEpisode()
		b.WriteString(errStyle.Render("Delete episode"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("Permanently delete %q from disk?\nThis cannot be undone.", ep.FileName))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("y / enter: confirm  ·  any other key: cancel"))
	}

	return searchPaneStyle.Render(b.String())
}

func formatSearchResult(r search.Result) string {
	if r.Kind == search.KindSeries {
		return fmt.Sprintf("[series]  %-40s %3d/%-3d", truncate(r.Series.Title, 40), r.Series.Watched, r.Series.Total)
	}
	return fmt.Sprintf("[episode] %s %-25s %s", statusicon.Icon(r.Episode.Status), truncate(r.Series.Title, 25), truncate(r.Episode.FileName, 55))
}

func progressBar(watched, total int) string {
	percent := 0
	if total > 0 {
		percent = watched * 100 / total
	}
	return percentBar(percent, barWidth)
}

func percentBar(percent, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := percent * width / 100
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}

func formatScanSummary(res scanner.Result) string {
	s := fmt.Sprintf("scanned: %d new series, %d new episodes, %d newly watched", res.NewSeries, res.NewEpisodes, res.NewlyWatched)
	if len(res.SkippedSeries) > 0 {
		s += fmt.Sprintf(" (%d skipped, unreadable)", len(res.SkippedSeries))
	}
	return s
}
