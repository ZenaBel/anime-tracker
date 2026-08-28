# anime-tracker

A single-binary CLI + TUI for tracking which anime episodes you've watched.

Point it at a directory where each top-level subfolder is a series and
contains `.mkv` episode files directly (no season subfolders). It tracks
episodes in a local SQLite database. Deleting an episode file (the usual
"I watched it, clean it up" workflow) is what marks it watched — no need to
tell the tool anything.

## Build

```sh
make build   # ./anime-tracker
make run     # build + run
make test    # go test ./...
make vet     # go vet ./...
make clean   # remove built binaries
```

Pure-Go SQLite driver (`modernc.org/sqlite`, no CGO), so it cross-compiles
cleanly. `make cross` builds linux/windows/darwin (amd64+arm64) binaries into
`dist/`.

## Usage

```sh
anime-tracker                            # launch the TUI (default when no command is given)
anime-tracker scan                       # scan the library, print what changed
anime-tracker list                       # list series with watch progress
anime-tracker list --sort <mode>         # az (default), za, added, watched
anime-tracker list <series>              # list episodes of one series, fuzzy-matched
anime-tracker play <query>               # fuzzy-find an episode, open it in the player
anime-tracker watch <query>              # manually mark an episode watched
anime-tracker unwatch <query>            # undo a watched mark
```

Global flags: `--dir` (library root, default: `$ANIME_TRACKER_DIR` or the
current directory), `--db` (sqlite file path, default: `$ANIME_TRACKER_DB`
or `~/.config/anime-tracker/anime.db`).

### TUI keys

`↑/↓` or `j/k` move within the focused pane · `←/→` or `h/l` switch panes ·
`enter` opens the selected episode in the player (series pane: focuses the
episode list) · `space` toggles an episode between watched/new · `r` rescans
the library · `s` cycles sort order (az → za → added → watched) · `q` quits.

### Choosing a player / MPV playback tracking

By default, `play` and the TUI's `enter` open an episode with the OS's
default-app opener (`xdg-open`/`open`/`start`) and immediately mark it
`watching` — actual "watched" detection still comes from the file being
deleted later (see below).

Set `ANIME_TRACKER_PLAYER=mpv` (or a full path whose base name is `mpv`) to
launch mpv directly over its JSON IPC socket instead. In that mode, the tool
tracks mpv's `end-file` event and marks the episode `watched` automatically
the moment it plays through to the end — no need to delete the file. If mpv
is quit or stopped early, the episode is left as `watching`. On Linux/macOS
this uses a unix socket; on Windows it falls back to a plain launch (no
IPC tracking, since mpv uses named pipes there instead).

Setting `ANIME_TRACKER_PLAYER` to anything else (e.g. `vlc`) just execs that
binary directly instead of going through the OS opener, with no playback
tracking.

For resuming an episode where you left off (not just knowing that you did),
mpv already has this built in independently of anime-tracker: add
`save-position-on-quit=yes` to `~/.config/mpv/mpv.conf` and mpv will
remember and restore the exact playback position per file on its own.

### Per-episode progress bar

With `ANIME_TRACKER_PLAYER=mpv`, if you quit before reaching the end, the
last known playback position and duration (observed live over IPC) are
saved, and a `watching` episode shows a mini progress bar next to it — e.g.
`◐ [##----] 33% Title - 05.mkv` — in both the TUI's episode pane and
`list <series>`. It updates each time you stop partway through an episode;
finishing one to EOF clears it (the `✓` icon already says "done").

## How "watched" is detected

On every scan, an episode file that used to be on disk but is no longer
there gets marked `watched` (with `started_at`/`finished_at` timestamps, if
missing, both set to the time of that scan). This is the primary detection
mechanism — it matches the real workflow of deleting a file after finishing
it. `watch`/`unwatch` and the TUI's `space` key exist for manual overrides,
but note a manual "unwatch" on an episode whose file is already gone will be
flipped back to `watched` on the next scan, since the scanner treats file
presence as the source of truth.

## Known limitations

- MPV IPC playback tracking (see above) only works on Linux/macOS; on
  Windows `ANIME_TRACKER_PLAYER=mpv` still launches mpv but without
  automatic watched-detection, since mpv's IPC there uses named pipes
  rather than unix sockets.
- Quality-variant duplicate folders (e.g. the same show downloaded as both
  `... [WEB-DL 1080p]` and `... [WEB-DL 1080p HEVC]`) are tracked as two
  independent series, since each top-level folder maps to one series.
