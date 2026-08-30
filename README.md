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
anime-tracker playlist <series-query>    # play all remaining episodes of a series as one mpv playlist
anime-tracker watch <query>              # manually mark an episode watched
anime-tracker unwatch <query>            # undo a watched mark
anime-tracker rename-series <query> <new-title>    # rename a series' folder + tracked title
anime-tracker delete-series <query>                # permanently delete a series' folder + all its files
anime-tracker rename-episode <query> <new-name>    # rename an episode's file (keeps its extension)
anime-tracker delete-episode <query>               # permanently delete one episode's file
anime-tracker config set <key> [value]             # configure remote qBittorrent/SSH (see below)
anime-tracker config show                          # print current config (password masked)
anime-tracker config unset <key>
anime-tracker download <series-query> <magnet-or-torrent-url>   # queue a download on the remote qBittorrent
anime-tracker sync-downloads [--dry-run]           # pull finished remote downloads into the library + rescan
anime-tracker rss [--all]                          # list RSS articles qBittorrent's RSS reader has fetched
anime-tracker rss-download <article-number> [series-query]   # download one listed article
```

The four `rename-*`/`delete-*` commands prompt for confirmation (`[y/N]`)
before touching anything; pass `-y`/`--yes` to skip it (e.g. for scripting).

Global flags: `--dir` (library root, default: `$ANIME_TRACKER_DIR` or the
current directory), `--db` (sqlite file path, default: `$ANIME_TRACKER_DB`
or `~/.config/anime-tracker/anime.db`).

### TUI keys

`↑/↓` or `j/k` move within the focused pane · `←/→` or `h/l` switch panes ·
`enter` opens the selected episode in the player (series pane: focuses the
episode list) · `space` toggles an episode between watched/new · `p` plays
the rest of the selected series as one playlist · `R` renames the selected
series/episode (on disk too) · `D` deletes it (on disk too, after a
confirmation prompt) · `r` rescans the library · `s` cycles sort order
(az → za → added → watched) · `/` opens search · `c` opens the settings
overlay (qBittorrent/SSH config — see below) · `g` opens the RSS overlay
(browse/download articles qBittorrent's RSS reader has fetched — see below)
· `S` runs `sync-downloads` (see below) and refreshes the panes if
anything came in · `q` quits.

### Renaming / deleting

`R` on a series or episode opens an inline rename prompt pre-filled with its
current name; `enter` confirms, `esc` cancels. `D` opens a confirmation
prompt ("permanently delete ... ? this cannot be undone") — only `y` or
`enter` proceeds, any other key cancels. Both act on the real file: renaming
a series renames its folder (and every episode's tracked path along with
it); renaming an episode renames its file and must keep its original
extension, since only files with it are picked up on rescan. Deleting either
one deletes the real file(s) from disk, not just the tracked entry — there's
no undo. The same four actions exist as CLI commands (`rename-series`,
`delete-series`, `rename-episode`, `delete-episode`), each with its own
`[y/N]` prompt (skip it with `-y`).

### Search

`/` opens a fuzzy search that, by default, matches both series titles and
episode file names at once, ranked together by match quality. `Tab` cycles
its scope (all → series only → episodes only → all) if you want to narrow
it; `↑/↓` move the selection, `enter` jumps straight to that series (or
that exact episode, within its series) in the normal panes, and `esc`
cancels back to wherever you were. Episode search loads fresh (all tracked
episodes, not just the currently selected series) each time you open it.

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

### Playing a series as a playlist

`anime-tracker playlist <series-query>` (or `p` in the TUI) launches mpv
once with every remaining episode of a series queued as a single playlist,
starting from the first not-yet-watched episode — so you don't have to
relaunch the player for each one. Each file's outcome is tracked
individually over the same mpv IPC mechanism as single-episode playback: an
episode mpv plays to EOF gets marked `watched` and mpv auto-advances to the
next one; if you quit partway through a file, that one keeps its partial
progress (shown as a `watching` progress bar) and playback stops there —
episodes further down the queue are left untouched. Requires
`ANIME_TRACKER_PLAYER=mpv`, since it's built on the same IPC socket as the
rest of the mpv integration.

### Remote qBittorrent + RSS

For a setup where the torrent client (qBittorrent) runs on a different
machine from the one running anime-tracker and holding the library — e.g. a
seedbox — `download` and `sync-downloads` close that loop over
qBittorrent's WebUI API and `rsync`/`ssh`.

**One-time setup:**

```sh
anime-tracker config set qbt.url https://seedbox.example.com:8080
anime-tracker config set qbt.username <your qBittorrent WebUI username>
anime-tracker config set qbt.password        # prompts, hidden input
anime-tracker config set remote.ssh_target <user>@seedbox.example.com   # or an ~/.ssh/config alias
anime-tracker config set remote.root /path/on/the/remote/disk           # where torrents anime-tracker submits are saved
anime-tracker config set qbt.insecure_tls true   # only if the WebUI uses a self-signed cert
```

Or do the same from the TUI: `c` opens a settings overlay listing all
keys, `enter` edits the selected one inline (`qbt.password`'s input is
masked as you type and never pre-filled), `x` clears one, `esc` closes it.
Same underlying storage either way.

These are stored in anime-tracker's own SQLite db (in plaintext, same
posture as `ANIME_TRACKER_PLAYER` in `.bashrc` — this is a personal,
single-user tool). Requires `rsync` and `ssh` on `PATH` locally, with
key-based SSH access to the remote host already working (`ssh
<remote.ssh_target>` should just connect, no password prompt) — this is the
one part of anime-tracker that isn't a dependency-free static binary.

**If qBittorrent runs in Docker** (common on a seedbox/NAS), `remote.root`
must be the path *as qBittorrent itself sees it* — its own container path
(e.g. `/downloads`), since that's what gets sent as `savepath` when this
tool adds a torrent. If that container path is bind-mounted from a
different real path on the host (e.g. `/downloads` in the container is
really `/home/user/qbittorrent/downloads` on the host SSH connects to),
set `remote.host_root` to that real host path too — `sync-downloads`
rewrites qBittorrent-reported paths from `remote.root` to `remote.host_root`
before handing them to rsync. Leave `remote.host_root` unset if qBittorrent
runs directly on the host (no container) — paths are then used as-is, and
`remote.root` alone is both the API's and the host's view. The tell that
this needs setting: `sync-downloads` failing with an rsync error like
`change_dir "<container path>" failed: No such file or directory` for a
path that doesn't match what you see over SSH.

**`anime-tracker download <series-query> <magnet-or-torrent-url>`** fuzzy-
resolves the series (same matching as `play`/`watch`), then tells the
remote qBittorrent to grab the given magnet link or `.torrent` URL, saved
to `remote.root` itself (flat, no per-series subfolder added) and tagged
`anime-tracker`. Flat because most release groups' torrents already wrap
their own files in a folder named after the release (which for a tracked
show is typically the series title itself) — adding `<remote.root>/<Series
Title>` on top used to leave a completed torrent nested two folders deep
locally (one layer from the save path, one already inside the torrent)
until `sync-downloads`' flattening cleaned it up — correctly, but
wastefully re-fetching the episode into that stray folder every run in the
meantime.

**RSS**: anime-tracker doesn't fetch or parse RSS feeds itself — subscribe
to feed URLs in qBittorrent's own WebUI (its RSS tab) as usual, and
anime-tracker reads what it's already fetched:

- `anime-tracker rss` lists unread articles across every subscribed feed
  (`--all` includes already-read ones too), newest first. `anime-tracker
  rss-download <article-number> [series-query]` sends that article's
  torrent to the remote qBittorrent — with `series-query` omitted, the
  series is guessed by finding which tracked series' title appears inside
  the article's title (e.g. "Frieren" inside `[SubsPlease] Frieren - 05
  [1080p]`) and asks for confirmation first (`-y` skips it).
- In the TUI, `g` opens the same thing as a two-pane overlay — feeds on the
  left (with unread counts, plus a synthetic "Unread" entry aggregating
  every feed, both mirroring qBittorrent's own WebUI RSS panel), that
  feed's articles on the right — navigated like the main series/episodes
  panes (`←/→`/`h/l` switch panes, `enter` on a feed focuses its articles).
  `enter` on an article moves to a series picker pre-filled with the same
  guess (type to search a different one if it's wrong, same fuzzy matching
  as `/` search), `enter` again downloads, `esc` steps back one level at a
  time.

Either way this ends up exactly like `download`: saved to `remote.root`
(flat) and tagged `anime-tracker`, so `sync-downloads` picks it up the
same way regardless of which path added it. qBittorrent's own RSS **Auto
Downloading** rules
work too, as a hands-off alternative to picking articles yourself — either
leave a rule's save path on qBittorrent's default (flat under
`remote.root`) and just add the tag `anime-tracker`, or point it at
`<remote.root>/<Series Folder Name>` (matching a local series folder name
exactly, case-insensitively) for a per-series subfolder — `sync-downloads`
handles both the same way. A brand-new show with no matching local folder
yet is fine either way; `sync-downloads` creates the folder on first sync.

**`anime-tracker sync-downloads`** looks up every `anime-tracker`-tagged
torrent, and for each one that's finished (`progress >= 1.0`, regardless of
whether it got there via `download` or an RSS rule): rsyncs it from the
remote host into the matching local series folder, flattens away any
nested batch-release folder qBittorrent may have kept (the scanner only
looks at direct `Series/*.mkv` files — a nested folder that happens to
share the series' own name, which some releases/qBittorrent produce even
for a single episode, is specifically handled so it doesn't get nested
inside itself locally), then removes the `anime-tracker` tag so it isn't
pulled again — the torrent itself is left alone, still seeding. Finishes
with a normal library scan. A torrent still mid-download is left untouched
and picked up on a later run; one whose transfer fails keeps its tag and
is retried next time too — including one stuck on a flattening conflict:
if the conflicting file turns out to be the exact same size (almost always
the same episode having arrived twice), the redundant copy is just
removed on the next attempt rather than failing forever; a genuinely
different file at that path is left as a real conflict for you to sort
out by hand. `--dry-run` lists what's finished without touching the
network or filesystem. `S` in the TUI runs the same thing (no `--dry-run`
there) and refreshes the series/episode panes right away if anything came
in.

**Which local folder a synced torrent lands in**: if its save path has a
per-series subfolder (`<remote.root>/<Series>` — from an Auto Downloading
rule with its own per-rule override), that segment is used directly. A
torrent saved flat (save path exactly equal to `remote.root` — always the
case for `download`/`rss-download`, and for any Auto Downloading rule left
on qBittorrent's plain default save location) has no folder segment to
read a series from; rather than dumping it into one shared local folder
literally named after `remote.root`'s own last segment (e.g. everything
piling into a local `downloads/`), `sync-downloads` instead guesses the
series from the torrent's own name — the same name-contains-series-title
check `rss-download`'s auto-guess uses — matched only against series you
already track locally. If nothing matches, that torrent is reported as
failed with an explicit reason instead of being silently misplaced; fix it
either by tracking the series locally first, or by giving that rule its
own save path (`<remote.root>/<Series Folder Name>`), then re-run
`sync-downloads`. `sync-downloads --dry-run` prints, per torrent, the
resolved series/local folder and whether rsync would merge cleanly or
nest — handy for troubleshooting a mismatch (two folder names can look
identical while differing byte-for-byte — stray whitespace, a Unicode
dash instead of a hyphen — which the `%q`-quoted output reveals).

While a file's actually transferring, both show live progress read from
rsync's own output — a `%d%%` figure plus transfer rate, updated on every
percentage change and at least once a second regardless (not spammed on
every rsync tick, but not frozen mid-percent on a large file either, where
one whole percent can be tens of MB) — instead of going silent for however
long a large episode takes. On the CLI this overwrites one line in place
(`fetching <name>: NN% (rate)`); in the TUI it's the status line under `S`.

There's no background polling — run `sync-downloads` whenever you want to
check (a cron job or a shell alias works fine for that).

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
- `download`/`sync-downloads` (see above) are the one feature that isn't
  dependency-free: they shell out to `rsync`/`ssh`, and assume the remote
  host is Linux-like and already reachable over key-based SSH.
