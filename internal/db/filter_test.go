package db

import (
	"context"
	"testing"
	"time"
)

func TestSeriesFilter_Matches(t *testing.T) {
	var (
		unstarted = SeriesProgress{Total: 3}
		started   = SeriesProgress{Total: 3, Watched: 1}
		inFlight  = SeriesProgress{Total: 3, Watching: 1}
		finished  = SeriesProgress{Total: 3, Watched: 3}
		empty     = SeriesProgress{}
		deleted   = SeriesProgress{Total: 3, Watched: 3, FilesDeleted: true}
		deletedUn = SeriesProgress{Total: 3, FilesDeleted: true}
	)

	cases := []struct {
		filter SeriesFilter
		want   map[string]bool
	}{
		{FilterAll, map[string]bool{"unstarted": true, "started": true, "inFlight": true, "finished": true, "empty": true}},
		{FilterUnwatched, map[string]bool{"unstarted": true, "started": true, "inFlight": true}},
		{FilterWatching, map[string]bool{"inFlight": true}},
		{FilterNotStarted, map[string]bool{"unstarted": true}},
		{FilterCompleted, map[string]bool{"finished": true}},
		{FilterDeleted, map[string]bool{"deleted": true, "deletedUn": true}},
	}
	all := map[string]SeriesProgress{
		"unstarted": unstarted, "started": started, "inFlight": inFlight,
		"finished": finished, "empty": empty, "deleted": deleted, "deletedUn": deletedUn,
	}
	for _, tc := range cases {
		for name, s := range all {
			if got := tc.filter.Matches(s); got != tc.want[name] {
				t.Errorf("%v.Matches(%s) = %v, want %v", tc.filter, name, got, tc.want[name])
			}
		}
	}
}

func TestFilterSeries_PreservesOrderAndDoesNotAlias(t *testing.T) {
	in := []SeriesProgress{
		{ID: 1, Total: 2},
		{ID: 2, Total: 2, Watched: 2},
		{ID: 3, Total: 2, FilesDeleted: true},
		{ID: 4, Total: 1},
	}
	got := FilterSeries(in, FilterUnwatched)
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 4 {
		t.Fatalf("FilterSeries ids = %v, want [1 4]", got)
	}
	got[0].ID = 99
	if in[0].ID != 1 {
		t.Error("FilterSeries result aliases the input slice")
	}
}

func TestSeriesFilter_NextCyclesThroughAll(t *testing.T) {
	seen := map[SeriesFilter]bool{}
	f := FilterAll
	for range seriesFilterNames {
		seen[f] = true
		f = f.Next()
	}
	if f != FilterAll {
		t.Errorf("after a full cycle got %v, want all", f)
	}
	if len(seen) != len(seriesFilterNames) {
		t.Errorf("cycle visited %d filters, want %d", len(seen), len(seriesFilterNames))
	}
}

func TestSeriesFilter_PrevIsInverseOfNext(t *testing.T) {
	for f := range seriesFilterNames {
		if got := f.Next().Prev(); got != f {
			t.Errorf("%v.Next().Prev() = %v, want %v", f, got, f)
		}
	}
	if got := FilterAll.Prev(); got != FilterDeleted {
		t.Errorf("all.Prev() = %v, want deleted", got)
	}
}

func TestParseSeriesFilter(t *testing.T) {
	for f, name := range seriesFilterNames {
		got, err := ParseSeriesFilter(name)
		if err != nil || got != f {
			t.Errorf("ParseSeriesFilter(%q) = %v, %v; want %v", name, got, err, f)
		}
	}
	if got, err := ParseSeriesFilter(""); err != nil || got != FilterAll {
		t.Errorf("ParseSeriesFilter(\"\") = %v, %v; want all", got, err)
	}
	if _, err := ParseSeriesFilter("bogus"); err == nil {
		t.Error("ParseSeriesFilter(bogus) should error")
	}
}

func TestListSeriesWithProgress_CountsWatching(t *testing.T) {
	store := newQueriesTestStore(t)
	ctx := context.Background()

	id, _, err := store.UpsertSeries(ctx, "Show", "/lib/Show")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.mkv", "b.mkv", "c.mkv"} {
		if _, err := store.UpsertEpisodeSeen(ctx, id, "/lib/Show/"+name, name, nil, 1, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if err := store.SetStatus(ctx, episodeIDByPath(t, store, "/lib/Show/a.mkv"), StatusWatching, &now, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStatus(ctx, episodeIDByPath(t, store, "/lib/Show/b.mkv"), StatusWatched, &now, &now); err != nil {
		t.Fatal(err)
	}

	got, err := store.ListSeriesWithProgress(ctx, SortAlphaAsc)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Total != 3 || got[0].Watched != 1 || got[0].Watching != 1 {
		t.Fatalf("got %+v, want Total=3 Watched=1 Watching=1", got)
	}
}
