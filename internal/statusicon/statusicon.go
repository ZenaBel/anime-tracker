// Package statusicon renders episode status as the icon shown in both the
// text `list` output and the TUI.
package statusicon

import "anime-tracker/internal/db"

func Icon(status string) string {
	switch status {
	case db.StatusWatching:
		return "◐"
	case db.StatusWatched:
		return "✓"
	default:
		return "●"
	}
}
