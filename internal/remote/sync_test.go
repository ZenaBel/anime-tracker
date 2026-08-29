package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
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

func TestFlattenDir_NameCollisionReported(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "01.mkv"))
	writeFile(t, filepath.Join(dir, "batch", "01.mkv"))

	err := FlattenDir(dir)
	if err == nil {
		t.Fatal("expected an error reporting the collision")
	}
	// The original top-level file must be untouched, and the nested one
	// left in place rather than silently dropped.
	if _, err := os.Stat(filepath.Join(dir, "batch", "01.mkv")); err != nil {
		t.Fatalf("colliding nested file should be left in place: %v", err)
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
