package tui

import (
	"testing"

	"anime-tracker/internal/db"
)

func TestFirstUnwatchedIndex(t *testing.T) {
	cases := []struct {
		name     string
		statuses []string
		want     int
	}{
		{"empty", nil, 0},
		{"all watched", []string{db.StatusWatched, db.StatusWatched}, 0},
		{"none watched", []string{db.StatusNew, db.StatusNew}, 0},
		{"some watched, next is new", []string{db.StatusWatched, db.StatusWatched, db.StatusNew, db.StatusNew}, 2},
		{"some watched, next is watching", []string{db.StatusWatched, db.StatusWatching, db.StatusNew}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var eps []db.Episode
			for _, s := range tc.statuses {
				eps = append(eps, db.Episode{Status: s})
			}
			if got := firstUnwatchedIndex(eps); got != tc.want {
				t.Errorf("firstUnwatchedIndex(%v) = %d, want %d", tc.statuses, got, tc.want)
			}
		})
	}
}

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
