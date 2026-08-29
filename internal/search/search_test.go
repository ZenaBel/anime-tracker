package search

import (
	"testing"

	"anime-tracker/internal/db"
)

func TestFindSeries(t *testing.T) {
	all := []db.SeriesProgress{
		{ID: 1, Title: "Attack on Titan"},
		{ID: 2, Title: "Frieren: Beyond Journey's End"},
		{ID: 3, Title: "Cowboy Bebop"},
	}

	got, ok := FindSeries(all, "frieren")
	if !ok {
		t.Fatal("expected a match")
	}
	if got.ID != 2 {
		t.Fatalf("FindSeries(\"frieren\") = %+v, want Frieren", got)
	}

	if _, ok := FindSeries(all, "zzz_no_such_series_zzz"); ok {
		t.Fatal("expected no match for nonsense query")
	}
}

func TestGuessSeriesForTitle(t *testing.T) {
	all := []db.SeriesProgress{
		{ID: 1, Title: "Attack on Titan"},
		{ID: 2, Title: "Frieren"},
		{ID: 3, Title: "Frieren: Beyond Journey's End"},
		{ID: 4, Title: "Cowboy Bebop"},
	}

	// The realistic case this exists for: a noisy release title (longer
	// than any series title) containing the series name as a substring —
	// FindSeries gets this backwards (it treats the noisy text as the
	// short query and title as the target, so it never matches).
	got, ok := GuessSeriesForTitle(all, "[SubsPlease] Cowboy Bebop - 05 [1080p]")
	if !ok || got.ID != 4 {
		t.Fatalf("GuessSeriesForTitle(release title) = (%+v, %v), want Cowboy Bebop", got, ok)
	}

	// The longer, more specific title wins over the shorter one it
	// contains as a prefix.
	got, ok = GuessSeriesForTitle(all, "[Group] Frieren: Beyond Journey's End - 12 [1080p]")
	if !ok || got.ID != 3 {
		t.Fatalf("GuessSeriesForTitle(ambiguous prefix) = (%+v, %v), want the longer Frieren title", got, ok)
	}

	if _, ok := GuessSeriesForTitle(all, "[Group] Some Other Show - 01 [1080p]"); ok {
		t.Fatal("expected no match for an unrelated title")
	}
}

func TestFindEpisode(t *testing.T) {
	all := []db.Episode{
		{ID: 1, FileName: "[SubsPlease] Jujutsu Kaisen - 05 [1080p].mkv"},
		{ID: 2, FileName: "[SubsPlease] Jujutsu Kaisen - 12 [1080p].mkv"},
	}

	got, ok := FindEpisode(all, "12")
	if !ok {
		t.Fatal("expected a match")
	}
	if got.ID != 2 {
		t.Fatalf("FindEpisode(\"12\") = %+v, want episode 12", got)
	}
}

func TestSearch(t *testing.T) {
	series := []db.SeriesProgress{
		{ID: 1, Title: "Frieren: Beyond Journey's End"},
		{ID: 2, Title: "Cowboy Bebop"},
	}
	episodes := []db.Episode{
		{ID: 10, SeriesID: 1, FileName: "[Erai-raws] Frieren - 05 [1080p].mkv"},
		{ID: 11, SeriesID: 2, FileName: "Cowboy Bebop - 01.mkv"},
	}

	t.Run("scope all finds both kinds", func(t *testing.T) {
		results := Search(series, episodes, "frieren", ScopeAll)
		if len(results) == 0 {
			t.Fatal("expected at least one match")
		}
		var sawSeries, sawEpisode bool
		for _, r := range results {
			if r.Kind == KindSeries && r.Series.ID == 1 {
				sawSeries = true
			}
			if r.Kind == KindEpisode && r.Episode.ID == 10 {
				sawEpisode = true
			}
		}
		if !sawSeries || !sawEpisode {
			t.Fatalf("results = %+v, want both the Frieren series and its episode", results)
		}
	})

	t.Run("scope series excludes episodes", func(t *testing.T) {
		results := Search(series, episodes, "frieren", ScopeSeries)
		for _, r := range results {
			if r.Kind != KindSeries {
				t.Fatalf("scope=series returned a non-series result: %+v", r)
			}
		}
		if len(results) != 1 {
			t.Fatalf("results = %+v, want exactly the Frieren series", results)
		}
	})

	t.Run("scope episodes excludes series", func(t *testing.T) {
		results := Search(series, episodes, "05", ScopeEpisodes)
		if len(results) != 1 || results[0].Kind != KindEpisode || results[0].Episode.ID != 10 {
			t.Fatalf("results = %+v, want exactly episode 10", results)
		}
	})

	t.Run("episode result carries its parent series", func(t *testing.T) {
		results := Search(series, episodes, "cowboy", ScopeEpisodes)
		if len(results) != 1 || results[0].Series.ID != 2 {
			t.Fatalf("results = %+v, want the episode tagged with Cowboy Bebop (series id 2)", results)
		}
	})

	t.Run("empty query returns everything in scope", func(t *testing.T) {
		results := Search(series, episodes, "", ScopeAll)
		if len(results) != len(series)+len(episodes) {
			t.Fatalf("got %d results, want %d", len(results), len(series)+len(episodes))
		}
	})
}

func TestNextScope(t *testing.T) {
	cases := []struct {
		in   Scope
		want Scope
	}{
		{ScopeAll, ScopeSeries},
		{ScopeSeries, ScopeEpisodes},
		{ScopeEpisodes, ScopeAll},
	}
	for _, tc := range cases {
		if got := NextScope(tc.in); got != tc.want {
			t.Errorf("NextScope(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
