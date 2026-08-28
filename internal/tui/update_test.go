package tui

import (
	"testing"

	"anime-tracker/internal/db"
)

func TestIndexByID(t *testing.T) {
	items := []db.Episode{{ID: 10}, {ID: 20}, {ID: 30}}
	keyFn := func(e db.Episode) int64 { return e.ID }

	if got := indexByID(items, 20, keyFn, 99); got != 1 {
		t.Errorf("indexByID found existing id: got %d, want 1", got)
	}
	if got := indexByID(items, 999, keyFn, 2); got != 2 {
		t.Errorf("indexByID missing id, in-bounds fallback: got %d, want 2", got)
	}
	if got := indexByID(items, 999, keyFn, 50); got != 2 {
		t.Errorf("indexByID missing id, out-of-bounds fallback clamped: got %d, want 2", got)
	}
	if got := indexByID([]db.Episode{}, 1, keyFn, 5); got != 0 {
		t.Errorf("indexByID empty list: got %d, want 0", got)
	}
}
