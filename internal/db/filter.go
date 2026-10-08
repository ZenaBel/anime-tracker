package db

import "fmt"

// SeriesFilter narrows a series list by watch state. Every filter except
// FilterDeleted also hides series whose files were deleted
// (SeriesProgress.FilesDeleted): there is nothing left on disk to watch,
// so they only show up when asked for explicitly.
type SeriesFilter int

const (
	FilterAll        SeriesFilter = iota // everything except files-deleted series
	FilterUnwatched                      // has at least one episode not yet watched
	FilterWatching                       // has an episode started but not finished
	FilterNotStarted                     // nothing watched or in progress yet
	FilterCompleted                      // every episode watched
	FilterDeleted                        // only files-deleted series
)

var seriesFilterNames = map[SeriesFilter]string{
	FilterAll:        "all",
	FilterUnwatched:  "unwatched",
	FilterWatching:   "watching",
	FilterNotStarted: "not-started",
	FilterCompleted:  "completed",
	FilterDeleted:    "deleted",
}

func (f SeriesFilter) String() string {
	if s, ok := seriesFilterNames[f]; ok {
		return s
	}
	return "all"
}

func ParseSeriesFilter(s string) (SeriesFilter, error) {
	if s == "" {
		return FilterAll, nil
	}
	for f, name := range seriesFilterNames {
		if name == s {
			return f, nil
		}
	}
	return FilterAll, fmt.Errorf("invalid filter %q (want all, unwatched, watching, not-started, completed, or deleted)", s)
}

// Next cycles to the following filter, wrapping from the last back to
// FilterAll.
func (f SeriesFilter) Next() SeriesFilter {
	return (f + 1) % (FilterDeleted + 1)
}

// Prev cycles to the preceding filter, wrapping from FilterAll back to the
// last one.
func (f SeriesFilter) Prev() SeriesFilter {
	return (f + FilterDeleted) % (FilterDeleted + 1)
}

func (f SeriesFilter) Matches(s SeriesProgress) bool {
	if f == FilterDeleted {
		return s.FilesDeleted
	}
	if s.FilesDeleted {
		return false
	}
	switch f {
	case FilterUnwatched:
		return s.Watched < s.Total
	case FilterWatching:
		return s.Watching > 0
	case FilterNotStarted:
		return s.Total > 0 && s.Watched == 0 && s.Watching == 0
	case FilterCompleted:
		return s.Total > 0 && s.Watched == s.Total
	default:
		return true
	}
}

// FilterSeries returns the series matching f, preserving order. It never
// aliases the input slice.
func FilterSeries(all []SeriesProgress, f SeriesFilter) []SeriesProgress {
	out := make([]SeriesProgress, 0, len(all))
	for _, s := range all {
		if f.Matches(s) {
			out = append(out, s)
		}
	}
	return out
}
