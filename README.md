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
anime-tracker                  # launch the TUI (default when no command is given)
anime-tracker scan             # scan the library, print what changed
anime-tracker list             # list series with watch progress
anime-tracker list <series>    # list episodes of one series, fuzzy-matched
anime-tracker play <query>     # fuzzy-find an episode, open it in the default player
anime-tracker watch <query>    # manually mark an episode watched
anime-tracker unwatch <query>  # undo a watched mark
```

Global flags: `--dir` (library root, default: `$ANIME_TRACKER_DIR` or the
current directory), `--db` (sqlite file path, default: `$ANIME_TRACKER_DB`
or `~/.config/anime-tracker/anime.db`).

### TUI keys

`↑/↓` or `j/k` move within the focused pane · `←/→` or `h/l` switch panes ·
`enter` opens the selected episode in the default player (series pane:
focuses the episode list) · `space` toggles an episode between watched/new ·
`r` rescans the library · `q` quits.

## How "watched" is detected

On every scan, an episode file that used to be on disk but is no longer
there gets marked `watched` (with `started_at`/`finished_at` timestamps, if
missing, both set to the time of that scan). This is the primary detection
mechanism — it matches the real workflow of deleting a file after finishing
it. `watch`/`unwatch` and the TUI's `space` key exist for manual overrides,
but note a manual "unwatch" on an episode whose file is already gone will be
flipped back to `watched` on the next scan, since the scanner treats file
presence as the source of truth.

## Not implemented (documented TODOs)

- **MPV IPC playback detection**: instead of relying on file deletion,
  launch MPV with `--input-ipc-server` and listen for the playback-finished
  event to mark an episode watched without needing to delete the file.
- **`ANIME_TRACKER_PLAYER` env var**: to exec a specific player binary
  directly instead of going through the OS's default-app opener
  (`xdg-open`/`open`/`start`).
