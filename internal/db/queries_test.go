package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newQueriesTestStore(t *testing.T) *Store {
	t.Helper()
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return NewStore(conn)
}

func episodeIDByPath(t *testing.T, store *Store, filePath string) int64 {
	t.Helper()
	var id int64
	if err := store.db.QueryRow(`SELECT id FROM episodes WHERE file_path = ?`, filePath).Scan(&id); err != nil {
		t.Fatalf("looking up episode id: %v", err)
	}
	return id
}

func TestListSeriesWithProgress_Sorting(t *testing.T) {
	store := newQueriesTestStore(t)
	ctx := context.Background()

	// Insert in a deliberately non-alphabetical order: Bravo, then Alpha,
	// then Charlie. "added" order relies on the AUTOINCREMENT id (not
	// created_at, whose second-level ticks can tie during a fast scan),
	// so this insertion order is itself the fixture for that case.
	if _, _, err := store.UpsertSeries(ctx, "Bravo", "/lib/Bravo"); err != nil {
		t.Fatal(err)
	}
	idA, _, err := store.UpsertSeries(ctx, "Alpha", "/lib/Alpha")
	if err != nil {
		t.Fatal(err)
	}
	idC, _, err := store.UpsertSeries(ctx, "Charlie", "/lib/Charlie")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Alpha and Charlie each get one watched episode; Bravo stays
	// untouched (never watched).
	if _, err := store.UpsertEpisodeSeen(ctx, idA, "/lib/Alpha/01.mkv", "01.mkv", nil, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	epA := episodeIDByPath(t, store, "/lib/Alpha/01.mkv")
	finA := base.Add(3 * time.Hour)
	if err := store.SetStatus(ctx, epA, StatusWatched, &finA, &finA); err != nil {
		t.Fatal(err)
	}

	if _, err := store.UpsertEpisodeSeen(ctx, idC, "/lib/Charlie/01.mkv", "01.mkv", nil, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	epC := episodeIDByPath(t, store, "/lib/Charlie/01.mkv")
	finC := base.Add(5 * time.Hour) // later than Alpha's
	if err := store.SetStatus(ctx, epC, StatusWatched, &finC, &finC); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		sort SortMode
		want []string
	}{
		{"alpha asc", SortAlphaAsc, []string{"Alpha", "Bravo", "Charlie"}},
		{"alpha desc", SortAlphaDesc, []string{"Charlie", "Bravo", "Alpha"}},
		{"added (newest first)", SortAdded, []string{"Charlie", "Alpha", "Bravo"}},
		{"last watched (most recent first, never-watched last)", SortLastWatched, []string{"Charlie", "Alpha", "Bravo"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := store.ListSeriesWithProgress(ctx, tc.sort)
			if err != nil {
				t.Fatal(err)
			}
			var titles []string
			for _, s := range got {
				titles = append(titles, s.Title)
			}
			if len(titles) != len(tc.want) {
				t.Fatalf("order = %v, want %v", titles, tc.want)
			}
			for i := range titles {
				if titles[i] != tc.want[i] {
					t.Fatalf("order = %v, want %v", titles, tc.want)
				}
			}
		})
	}
}

func TestParseSortMode(t *testing.T) {
	cases := []struct {
		in      string
		want    SortMode
		wantErr bool
	}{
		{"", SortAlphaAsc, false},
		{"az", SortAlphaAsc, false},
		{"za", SortAlphaDesc, false},
		{"added", SortAdded, false},
		{"watched", SortLastWatched, false},
		{"bogus", SortAlphaAsc, true},
	}
	for _, tc := range cases {
		got, err := ParseSortMode(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseSortMode(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
		}
		if got != tc.want {
			t.Errorf("ParseSortMode(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestEpisodeProgressPercent(t *testing.T) {
	f := func(v float64) *float64 { return &v }

	cases := []struct {
		name     string
		pos, dur *float64
		wantPct  int
		wantOK   bool
	}{
		{"no data", nil, nil, 0, false},
		{"no duration", f(100), nil, 0, false},
		{"zero duration", f(100), f(0), 0, false},
		{"halfway", f(600), f(1200), 50, true},
		{"clamped over 100", f(1300), f(1200), 100, true},
		{"just started", f(0), f(1200), 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ep := Episode{ResumePositionSecs: tc.pos, DurationSecs: tc.dur}
			pct, ok := ep.ProgressPercent()
			if ok != tc.wantOK || pct != tc.wantPct {
				t.Errorf("ProgressPercent() = (%d, %v), want (%d, %v)", pct, ok, tc.wantPct, tc.wantOK)
			}
		})
	}
}

func TestSetPlaybackProgress(t *testing.T) {
	store := newQueriesTestStore(t)
	ctx := context.Background()

	seriesID, _, err := store.UpsertSeries(ctx, "Frieren", "/lib/Frieren")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertEpisodeSeen(ctx, seriesID, "/lib/Frieren/01.mkv", "01.mkv", nil, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	epID := episodeIDByPath(t, store, "/lib/Frieren/01.mkv")

	if err := store.SetPlaybackProgress(ctx, epID, 300, 1200); err != nil {
		t.Fatal(err)
	}
	eps, err := store.ListEpisodesBySeries(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	pct, ok := eps[0].ProgressPercent()
	if !ok || pct != 25 {
		t.Fatalf("after setting progress: ProgressPercent() = (%d, %v), want (25, true)", pct, ok)
	}

	// durationSecs <= 0 clears the recorded progress.
	if err := store.SetPlaybackProgress(ctx, epID, 0, 0); err != nil {
		t.Fatal(err)
	}
	eps, err = store.ListEpisodesBySeries(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if eps[0].ResumePositionSecs != nil || eps[0].DurationSecs != nil {
		t.Fatalf("progress not cleared: %+v", eps[0])
	}
}
