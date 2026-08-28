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
