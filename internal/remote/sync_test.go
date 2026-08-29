package remote

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRsyncArgs(t *testing.T) {
	got := buildRsyncArgs("user@seedbox", "/downloads/Frieren - 05 [1080p].mkv", "/lib/Frieren")
	want := []string{"-avz", "-s", "-e", "ssh", "user@seedbox:/downloads/Frieren - 05 [1080p].mkv", "/lib/Frieren" + string(filepath.Separator)}
	if len(got) != len(want) {
		t.Fatalf("buildRsyncArgs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("buildRsyncArgs()[%d] = %q, want %q", i, got[i], want[i])
		}
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
