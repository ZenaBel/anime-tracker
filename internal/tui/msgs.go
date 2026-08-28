package tui

import (
	"anime-tracker/internal/db"
	"anime-tracker/internal/scanner"
)

type seriesLoadedMsg struct {
	series []db.SeriesProgress
	err    error
}

type episodesLoadedMsg struct {
	episodes []db.Episode
	err      error
}

type scanCompleteMsg struct {
	result scanner.Result
	err    error
}

type actionDoneMsg struct {
	err error
}
