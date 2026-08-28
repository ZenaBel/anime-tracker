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
