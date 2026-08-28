package search

import (
	"github.com/sahilm/fuzzy"

	"anime-tracker/internal/db"
)

type seriesSource []db.SeriesProgress

func (s seriesSource) String(i int) string { return s[i].Title }
func (s seriesSource) Len() int            { return len(s) }

type episodeSource []db.Episode

func (e episodeSource) String(i int) string { return e[i].FileName }
func (e episodeSource) Len() int            { return len(e) }

func FindSeries(all []db.SeriesProgress, query string) (db.SeriesProgress, bool) {
	matches := fuzzy.FindFrom(query, seriesSource(all))
	if len(matches) == 0 {
		return db.SeriesProgress{}, false
	}
	return all[matches[0].Index], true
}

func FindEpisode(all []db.Episode, query string) (db.Episode, bool) {
	matches := fuzzy.FindFrom(query, episodeSource(all))
	if len(matches) == 0 {
		return db.Episode{}, false
	}
	return all[matches[0].Index], true
}

// Scope narrows a combined Search to series only, episodes only, or both.
type Scope int

const (
	ScopeAll Scope = iota
	ScopeSeries
	ScopeEpisodes
)

func (s Scope) String() string {
	switch s {
	case ScopeSeries:
		return "series"
	case ScopeEpisodes:
		return "episodes"
	default:
		return "all"
	}
}

// NextScope cycles All -> Series -> Episodes -> All.
func NextScope(s Scope) Scope {
	switch s {
	case ScopeAll:
		return ScopeSeries
	case ScopeSeries:
		return ScopeEpisodes
	default:
		return ScopeAll
	}
}

type Kind int

const (
	KindSeries Kind = iota
	KindEpisode
)

// Result is one combined search hit. Episode is only meaningful when
// Kind == KindEpisode; Series is always populated (an episode result
// carries its parent series too, for display/navigation).
type Result struct {
	Kind    Kind
	Series  db.SeriesProgress
	Episode db.Episode
}

type combinedSource []Result

func (c combinedSource) String(i int) string {
	r := c[i]
	if r.Kind == KindSeries {
		return r.Series.Title
	}
	return r.Series.Title + " " + r.Episode.FileName
}
func (c combinedSource) Len() int { return len(c) }

// Search fuzzy-matches query against series titles and/or episode file
// names (scoped by seriesTitle too, so "frieren 05" finds the episode even
// though "05" alone isn't in the filename's series name) depending on
// scope, ranked by match quality. An empty query returns everything in
// scope, unranked, in series/episode list order.
func Search(allSeries []db.SeriesProgress, allEpisodes []db.Episode, query string, scope Scope) []Result {
	seriesByID := make(map[int64]db.SeriesProgress, len(allSeries))
	for _, s := range allSeries {
		seriesByID[s.ID] = s
	}

	var items []Result
	if scope != ScopeEpisodes {
		for _, s := range allSeries {
			items = append(items, Result{Kind: KindSeries, Series: s})
		}
	}
	if scope != ScopeSeries {
		for _, ep := range allEpisodes {
			items = append(items, Result{Kind: KindEpisode, Series: seriesByID[ep.SeriesID], Episode: ep})
		}
	}

	if query == "" {
		return items
	}

	matches := fuzzy.FindFrom(query, combinedSource(items))
	out := make([]Result, len(matches))
	for i, m := range matches {
		out[i] = items[m.Index]
	}
	return out
}
