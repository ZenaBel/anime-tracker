package remote

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"anime-tracker/internal/db"
	"anime-tracker/internal/qbt"
)

func TestBuildRsyncArgs(t *testing.T) {
	got := buildRsyncArgs("user@seedbox", "/downloads/Frieren - 05 [1080p].mkv", "/lib/Frieren")
	want := []string{
		"-avz", "-s", "--info=progress2", "--outbuf=L", "-e", "ssh",
		"user@seedbox:/downloads/Frieren - 05 [1080p].mkv",
		"/lib/Frieren" + string(filepath.Separator),
	}
	if len(got) != len(want) {
		t.Fatalf("buildRsyncArgs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("buildRsyncArgs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// Regression test for a real repro: a torrent's content_path was itself a
// folder sharing the series' own name (e.g. "Futsutsuka na Akujo dewa
// Gozaimasu ga - AniLiberty [WEB-DL 1080p HEVC]" both as the series folder
// and as content_path's own last segment) — without the trailing slash,
// rsync nests that whole folder inside itself locally instead of merging
// its contents in.
func TestBuildRsyncArgs_SameNameFolderGetsTrailingSlash(t *testing.T) {
	got := buildRsyncArgs(
		"user@seedbox",
		"/downloads/Futsutsuka na Akujo dewa Gozaimasu ga - AniLiberty [WEB-DL 1080p HEVC]/Futsutsuka na Akujo dewa Gozaimasu ga - AniLiberty [WEB-DL 1080p HEVC]",
		"/lib/Futsutsuka na Akujo dewa Gozaimasu ga - AniLiberty [WEB-DL 1080p HEVC]",
	)
	wantSrc := "user@seedbox:/downloads/Futsutsuka na Akujo dewa Gozaimasu ga - AniLiberty [WEB-DL 1080p HEVC]/Futsutsuka na Akujo dewa Gozaimasu ga - AniLiberty [WEB-DL 1080p HEVC]/"
	if got[len(got)-2] != wantSrc {
		t.Fatalf("buildRsyncArgs() source = %q, want %q (trailing slash so rsync merges contents instead of nesting)", got[len(got)-2], wantSrc)
	}
}

func TestParseRsyncProgress(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantOK   bool
		wantPct  int
		wantRate string
	}{
		{
			"typical progress2 line",
			"      1,234,567  45%   12.34MB/s    0:00:12 (xfr#1, to-chk=0/1)",
			true, 45, "12.34MB/s",
		},
		{"100% completion line", "  1,853,958,835 100%  118.20MB/s    0:00:14 (xfr#1, to-chk=0/1)", true, 100, "118.20MB/s"},
		{"non-progress line: file list header", "receiving incremental file list", false, 0, ""},
		{"non-progress line: blank", "", false, 0, ""},
		{"non-progress line: summary footer", "sent 8 bytes  received 8 bytes  32.00 bytes/sec", false, 0, ""},
		{
			// Regression: real repro on a uk_UA-locale machine — period as
			// thousands separator, comma as decimal point — where rsync
			// itself (not anime-tracker) formats the line this way.
			"locale-formatted line (period thousands, comma decimal)",
			"   32.768.800  45%   12,34MB/s    0:00:12 (xfr#1, to-chk=0/1)",
			true, 45, "12,34MB/s",
		},
		{
			"locale-formatted line, byte count small enough to have no separator yet",
			"          0   0%    0,00kB/s    0:00:00",
			true, 0, "0,00kB/s",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := parseRsyncProgress(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("parseRsyncProgress(%q) ok = %v, want %v", tc.line, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if p.Percent != tc.wantPct || p.Rate != tc.wantRate {
				t.Errorf("parseRsyncProgress(%q) = %+v, want {%d %s}", tc.line, p, tc.wantPct, tc.wantRate)
			}
		})
	}
}

func TestResolveSeriesNameForSync(t *testing.T) {
	allSeries := []db.SeriesProgress{
		{ID: 1, Title: "Frieren"},
		{ID: 2, Title: "Pyl Myobiusa"},
	}

	t.Run("proper per-series subfolder used as-is", func(t *testing.T) {
		name, ok := resolveSeriesNameForSync("/downloads/Frieren", "/downloads/Frieren", "/downloads", "[SubsPlease] Frieren - 05 [1080p]", allSeries)
		if !ok || name != "Frieren" {
			t.Fatalf("resolveSeriesNameForSync() = (%q, %v), want (Frieren, true)", name, ok)
		}
	})

	t.Run("containerRoot unset: falls back to plain basename regardless", func(t *testing.T) {
		name, ok := resolveSeriesNameForSync("/downloads", "/downloads", "", "[SubsPlease] Frieren - 05 [1080p]", allSeries)
		if !ok || name != "downloads" {
			t.Fatalf("resolveSeriesNameForSync() = (%q, %v), want (downloads, true) — no containerRoot configured means no guard", name, ok)
		}
	})

	t.Run("saved flat (no subfolder): guesses from torrent name", func(t *testing.T) {
		name, ok := resolveSeriesNameForSync("/downloads", "/downloads/Pyl Myobiusa - 08 [1080p]", "/downloads", "[SubsPlease] Pyl Myobiusa - 08 [1080p]", allSeries)
		if !ok || name != "Pyl Myobiusa" {
			t.Fatalf("resolveSeriesNameForSync() = (%q, %v), want (Pyl Myobiusa, true)", name, ok)
		}
	})

	t.Run("saved flat, trailing slash on containerRoot still matches", func(t *testing.T) {
		name, ok := resolveSeriesNameForSync("/downloads", "/downloads/Frieren - 05", "/downloads/", "[SubsPlease] Frieren - 05 [1080p]", allSeries)
		if !ok || name != "Frieren" {
			t.Fatalf("resolveSeriesNameForSync() = (%q, %v), want (Frieren, true)", name, ok)
		}
	})

	t.Run("saved flat, no tracked series matches but qBittorrent made a content subfolder: seeds a new series from it", func(t *testing.T) {
		name, ok := resolveSeriesNameForSync("/downloads", "/downloads/Some Unrelated Show - AniLiberty [WEB-DL 1080p]", "/downloads", "[SubsPlease] Some Unrelated Show - 01 [1080p]", allSeries)
		if !ok || name != "Some Unrelated Show - AniLiberty [WEB-DL 1080p]" {
			t.Fatalf("resolveSeriesNameForSync() = (%q, %v), want (%q, true) — content subfolder name used to seed a brand-new series", name, ok, "Some Unrelated Show - AniLiberty [WEB-DL 1080p]")
		}
	})

	t.Run("saved flat, content is a bare file with no subfolder at all: fails clearly", func(t *testing.T) {
		_, ok := resolveSeriesNameForSync("/downloads", "/downloads/Some Unrelated Show - 01.mkv", "/downloads", "[SubsPlease] Some Unrelated Show - 01 [1080p]", allSeries)
		if ok {
			t.Fatal("expected resolveSeriesNameForSync to fail: content_path is a flat file, not a well-formed series folder name")
		}
	})

	t.Run("saved flat, content_path also equals containerRoot (no info at all): fails clearly", func(t *testing.T) {
		_, ok := resolveSeriesNameForSync("/downloads", "/downloads", "/downloads", "[SubsPlease] Some Unrelated Show - 01 [1080p]", allSeries)
		if ok {
			t.Fatal("expected resolveSeriesNameForSync to fail when neither a series match nor a usable content subfolder exists")
		}
	})
}

func TestPlanSync(t *testing.T) {
	allSeries := []db.SeriesProgress{{ID: 1, Title: "Frieren"}}
	libRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(libRoot, "Frieren"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("proper subfolder, content_path nested with matching name: would merge", func(t *testing.T) {
		tr := qbt.Torrent{
			Name:        "[SubsPlease] Frieren - 05 [1080p]",
			SavePath:    "/downloads/Frieren",
			ContentPath: "/downloads/Frieren/Frieren",
		}
		plan := PlanSync(tr, "/downloads", "", libRoot, allSeries)
		if !plan.OK || !plan.WouldMergeIntoDir {
			t.Fatalf("PlanSync() = %+v, want OK and WouldMergeIntoDir", plan)
		}
	})

	// A regular batch folder named after the release, not the series — the
	// intended "gets nested then FlattenDir sorts it out" case, not a bug.
	t.Run("content_path basename differs from series name: no merge, by design", func(t *testing.T) {
		tr := qbt.Torrent{
			Name:        "[Group] Frieren Batch 01-10",
			SavePath:    "/downloads/Frieren",
			ContentPath: "/downloads/Frieren/[Group] Frieren Batch 01-10",
		}
		plan := PlanSync(tr, "/downloads", "", libRoot, allSeries)
		if !plan.OK || plan.WouldMergeIntoDir {
			t.Fatalf("PlanSync() = %+v, want OK and !WouldMergeIntoDir", plan)
		}
	})

	// Regression guard: two folder names that render identically can still
	// differ byte-for-byte — here an ASCII hyphen vs a Unicode en dash —
	// which silently defeats the same-name merge. This is exactly the kind
	// of mismatch RemoteBasename/LocalDirBasename (printed with %q by
	// `sync-downloads --dry-run`) is meant to surface.
	t.Run("visually identical but byte-different basenames: no merge", func(t *testing.T) {
		if err := os.Mkdir(filepath.Join(libRoot, "Black Torch – AniLiberty"), 0o755); err != nil {
			t.Fatal(err)
		}
		tr := qbt.Torrent{
			Name:        "Black Torch - AniLiberty [WEBRip 1080p HEVC]",
			SavePath:    "/downloads/Black Torch – AniLiberty",                          // en dash
			ContentPath: "/downloads/Black Torch – AniLiberty/Black Torch - AniLiberty", // hyphen
		}
		plan := PlanSync(tr, "/downloads", "", libRoot, allSeries)
		if !plan.OK {
			t.Fatalf("PlanSync() = %+v, want OK", plan)
		}
		if plan.WouldMergeIntoDir {
			t.Fatalf("PlanSync() = %+v, want !WouldMergeIntoDir (basenames differ by dash character despite looking identical)", plan)
		}
		if plan.RemoteBasename == plan.LocalDirBasename {
			t.Fatalf("RemoteBasename and LocalDirBasename should differ: %q vs %q", plan.RemoteBasename, plan.LocalDirBasename)
		}
	})
}

func TestRemapPath(t *testing.T) {
	cases := []struct {
		name                           string
		rpath, containerRoot, hostRoot string
		want                           string
	}{
		{
			"docker bind-mount, real repro shape",
			"/downloads/Futsutsuka na Akujo dewa Gozaimasu ga - AniLiberty [WEB-DL 1080p HEVC]/ep01.mkv",
			"/downloads", "/home/nitro/qbittorrent/downloads",
			"/home/nitro/qbittorrent/downloads/Futsutsuka na Akujo dewa Gozaimasu ga - AniLiberty [WEB-DL 1080p HEVC]/ep01.mkv",
		},
		{"path equals containerRoot exactly", "/downloads", "/downloads", "/home/nitro/downloads", "/home/nitro/downloads"},
		{"neither root configured: unchanged", "/downloads/Show/ep01.mkv", "", "", "/downloads/Show/ep01.mkv"},
		{"only containerRoot configured: unchanged", "/downloads/Show/ep01.mkv", "/downloads", "", "/downloads/Show/ep01.mkv"},
		{"only hostRoot configured: unchanged", "/downloads/Show/ep01.mkv", "", "/host/downloads", "/downloads/Show/ep01.mkv"},
		{"path not under containerRoot: unchanged", "/other/Show/ep01.mkv", "/downloads", "/host/downloads", "/other/Show/ep01.mkv"},
		{"prefix collision without separator: unchanged", "/downloads2/Show/ep01.mkv", "/downloads", "/host/downloads", "/downloads2/Show/ep01.mkv"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := remapPath(tc.rpath, tc.containerRoot, tc.hostRoot); got != tc.want {
				t.Errorf("remapPath(%q, %q, %q) = %q, want %q", tc.rpath, tc.containerRoot, tc.hostRoot, got, tc.want)
			}
		})
	}
}

// Regression test for a real repro: a large file sat at "0%" with a
// frozen "0.00kB/s" the whole time it was actually transferring, because
// the old percent-only throttle never re-emitted while the whole-number
// percentage stayed the same — which for a large file can be a long
// stretch of real time. shouldEmitProgress must re-emit periodically even
// with no percentage change, so the shown rate stays live.
func TestShouldEmitProgress(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name                 string
		percent, lastPercent int
		now, lastEmit        time.Time
		want                 bool
	}{
		{"first ever update (lastEmit zero)", 0, -1, base, time.Time{}, true},
		{"percent changed, no time elapsed", 1, 0, base, base, true},
		{"percent unchanged, <1s elapsed: suppressed", 0, 0, base.Add(500 * time.Millisecond), base, false},
		{"percent unchanged, >=1s elapsed: re-emit for a live rate", 0, 0, base.Add(1500 * time.Millisecond), base, true},
		{"percent unchanged, exactly 1s elapsed: re-emit", 5, 5, base.Add(time.Second), base, true},
		{"reaching 100% always emits, even with no time elapsed", 100, 100, base, base, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldEmitProgress(tc.percent, tc.lastPercent, tc.now, tc.lastEmit); got != tc.want {
				t.Errorf("shouldEmitProgress(%d, %d, ...) = %v, want %v", tc.percent, tc.lastPercent, got, tc.want)
			}
		})
	}
}

// TestStreamRsyncProgress_Throttling verifies the full scan+parse+throttle
// pipeline Fetch uses internally, fed real-shaped rsync --info=progress2
// output (rsync repeats the same percentage many times a second — this
// must only fire onProgress once per distinct percentage change).
func TestStreamRsyncProgress_Throttling(t *testing.T) {
	sample := strings.Join([]string{
		"sending incremental file list",
		"Mebius Dust - AniLiberty [WEB-DL 1080p HEVC]/ep01.mkv",
		"      1,234,567   5%   10.00MB/s    0:02:50",
		"      2,345,678   5%   10.10MB/s    0:02:48", // same 5% again — must not re-fire
		"      3,456,789  10%   10.20MB/s    0:02:40",
		"     12,345,678  45%   12.34MB/s    0:00:12",
		"     12,345,679  45%   12.34MB/s    0:00:12", // same 45% again — must not re-fire
		"  1,853,958,835 100%  118.20MB/s    0:00:14 (xfr#1, to-chk=0/1)",
		"",
		"sent 1,234 bytes  received 987,654,321 bytes  1,234,567.89 bytes/sec",
		"total size is 1,853,958,835  speedup is 1.00",
	}, "\n")

	var got []FetchProgress
	lastLine := streamRsyncProgress(strings.NewReader(sample), func(p FetchProgress) {
		got = append(got, p)
	})

	want := []FetchProgress{
		{Percent: 5, Rate: "10.00MB/s"},
		{Percent: 10, Rate: "10.20MB/s"},
		{Percent: 45, Rate: "12.34MB/s"},
		{Percent: 100, Rate: "118.20MB/s"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d progress updates, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("update[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	wantLastLine := "total size is 1,853,958,835  speedup is 1.00"
	if lastLine != wantLastLine {
		t.Errorf("lastLine = %q, want %q", lastLine, wantLastLine)
	}
}

// Regression test for a real repro, confirmed by capturing rsync's actual
// output byte-for-byte: --info=progress2 redraws its line in place using a
// bare '\r' with no '\n' at all in between — only the line for a
// *finished* file ends in '\n'. With the old \n-only bufio.ScanLines split,
// every intermediate '\r'-joined update before that final '\n' was
// invisible — onProgress never fired until a whole file finished — which
// is what "stuck at 0%/0.00kB/s" looked like even while bytes were moving.
func TestStreamRsyncProgress_CRSeparatedUpdates(t *testing.T) {
	sample := "sending incremental file list\n" +
		"big.bin\n" +
		"\r      1,234,567   1%    1.00MB/s    0:00:50" +
		"\r      2,345,678   2%    1.10MB/s    0:00:48" +
		"\r      3,456,789   3%    1.20MB/s    0:00:46" +
		"\r  1,853,958,835 100%  118.20MB/s    0:00:14 (xfr#1, to-chk=0/1)\n" +
		"\n" +
		"sent 1,234 bytes  received 987,654,321 bytes  1,234,567.89 bytes/sec\n"

	var got []FetchProgress
	streamRsyncProgress(strings.NewReader(sample), func(p FetchProgress) {
		got = append(got, p)
	})

	want := []FetchProgress{
		{Percent: 1, Rate: "1.00MB/s"},
		{Percent: 2, Rate: "1.10MB/s"},
		{Percent: 3, Rate: "1.20MB/s"},
		{Percent: 100, Rate: "118.20MB/s"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d progress updates, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("update[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// Without a '\n' in sight, the old \n-only split would keep accumulating
// one giant token past bufio's default 64KB cap and Scan would stop dead
// (silently — its error was never checked), while rsync kept writing to
// the now-unread pipe and blocked, stalling the whole transfer. This feeds
// well over 64KB of '\r'-joined updates with no '\n' anywhere and checks
// updates keep flowing throughout, not just up to some silent cutoff.
func TestStreamRsyncProgress_CRSeparatedUpdates_ExceedsScannerBuffer(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("sending incremental file list\nbig.bin\n")
	const lines = 3000 // ~90KB of '\r'-joined content, no '\n' until the end
	for i := 1; i <= lines; i++ {
		fmt.Fprintf(&sb, "\r      %d   %d%%    1.00MB/s    0:00:50", i, i%100)
	}
	sb.WriteString("\r  1,853,958,835 100%  118.20MB/s    0:00:14 (xfr#1, to-chk=0/1)\n")

	var got []FetchProgress
	streamRsyncProgress(strings.NewReader(sb.String()), func(p FetchProgress) {
		got = append(got, p)
	})

	if len(got) == 0 {
		t.Fatal("expected progress updates even past the old 64KB single-token limit, got none")
	}
	if last := got[len(got)-1]; last.Percent != 100 {
		t.Fatalf("last update = %+v, want the final 100%% line to be seen", last)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	writeFileContent(t, path, "data")
}

func writeFileContent(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFlattenDir_NestedBatchFolder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "[Group] Release Batch", "Frieren - 01 [1080p].mkv"))
	writeFile(t, filepath.Join(dir, "[Group] Release Batch", "Frieren - 02 [1080p].mkv"))

	if err := FlattenDir(dir); err != nil {
		t.Fatalf("FlattenDir() error = %v", err)
	}

	for _, name := range []string{"Frieren - 01 [1080p].mkv", "Frieren - 02 [1080p].mkv"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s at top level: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "[Group] Release Batch")); !os.IsNotExist(err) {
		t.Errorf("expected the now-empty batch folder to be removed")
	}
}

// A leftover non-.mkv file (e.g. a batch release's .nfo/.jpg) is not moved
// or deleted, so its folder is deliberately left behind rather than
// silently losing that file.
func TestFlattenDir_LeavesNonMkvSiblingsInPlace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "[Group] Release Batch", "Frieren - 01 [1080p].mkv"))
	writeFile(t, filepath.Join(dir, "[Group] Release Batch", "info.nfo"))

	if err := FlattenDir(dir); err != nil {
		t.Fatalf("FlattenDir() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "Frieren - 01 [1080p].mkv")); err != nil {
		t.Errorf("expected the mkv at top level: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "[Group] Release Batch", "info.nfo")); err != nil {
		t.Errorf("expected info.nfo left in place: %v", err)
	}
}

func TestFlattenDir_AlreadyFlat(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Frieren - 01 [1080p].mkv"))

	if err := FlattenDir(dir); err != nil {
		t.Fatalf("FlattenDir() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Frieren - 01 [1080p].mkv")); err != nil {
		t.Fatalf("file should still be present: %v", err)
	}
}

func TestFlattenDir_DifferentSizeCollisionReported(t *testing.T) {
	dir := t.TempDir()
	writeFileContent(t, filepath.Join(dir, "01.mkv"), "the real episode, much bigger")
	writeFileContent(t, filepath.Join(dir, "batch", "01.mkv"), "diff")

	err := FlattenDir(dir)
	if err == nil {
		t.Fatal("expected an error reporting the collision")
	}
	// A genuine size mismatch: neither copy is touched — never guess which
	// one is "right".
	if _, err := os.Stat(filepath.Join(dir, "batch", "01.mkv")); err != nil {
		t.Fatalf("colliding nested file should be left in place: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "01.mkv")); err != nil {
		t.Fatalf("original top-level file should be left in place: %v", err)
	}
}

// Regression test: a torrent re-delivering an episode that already exists
// locally (e.g. via buildRsyncArgs's same-name-folder case, or any other
// path that lands a duplicate) must not get FlattenDir permanently stuck
// reporting the exact same collision forever — a same-size collision is
// treated as a harmless duplicate and cleaned up instead.
func TestFlattenDir_SameSizeDuplicateRemoved(t *testing.T) {
	dir := t.TempDir()
	writeFileContent(t, filepath.Join(dir, "01.mkv"), "identical content!!")
	writeFileContent(t, filepath.Join(dir, "Series Name", "01.mkv"), "identical content!!")

	if err := FlattenDir(dir); err != nil {
		t.Fatalf("FlattenDir() error = %v, want nil (same-size duplicate should be silently cleaned up)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "01.mkv")); err != nil {
		t.Fatalf("original top-level file should survive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Series Name")); !os.IsNotExist(err) {
		t.Fatalf("nested duplicate and its now-empty folder should be gone")
	}
}

func TestResolveLocalSeriesDir_ExistingCaseInsensitive(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Frieren"), 0o755); err != nil {
		t.Fatal(err)
	}

	path, isNew, err := ResolveLocalSeriesDir(root, "frieren")
	if err != nil {
		t.Fatal(err)
	}
	if isNew {
		t.Fatal("expected an existing series to not be reported as new")
	}
	if path != filepath.Join(root, "Frieren") {
		t.Fatalf("path = %q, want the existing Frieren folder's exact casing", path)
	}
}

func TestResolveLocalSeriesDir_New(t *testing.T) {
	root := t.TempDir()

	path, isNew, err := ResolveLocalSeriesDir(root, "Brand New Show")
	if err != nil {
		t.Fatal(err)
	}
	if !isNew {
		t.Fatal("expected a series with no matching folder to be reported as new")
	}
	if path != filepath.Join(root, "Brand New Show") {
		t.Fatalf("path = %q, want %q", path, filepath.Join(root, "Brand New Show"))
	}
}
