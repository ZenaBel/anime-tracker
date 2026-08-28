package player

import "testing"

func TestBuild(t *testing.T) {
	cases := []struct {
		goos     string
		wantPath string
		wantArgs []string
	}{
		{"linux", "xdg-open", []string{"xdg-open", "/tmp/ep.mkv"}},
		{"darwin", "open", []string{"open", "/tmp/ep.mkv"}},
		{"windows", "cmd", []string{"cmd", "/c", "start", "", "/tmp/ep.mkv"}},
	}

	for _, tc := range cases {
		t.Run(tc.goos, func(t *testing.T) {
			cmd, err := build(tc.goos, "/tmp/ep.mkv")
			if err != nil {
				t.Fatalf("build(%q) returned error: %v", tc.goos, err)
			}
			if len(cmd.Args) != len(tc.wantArgs) {
				t.Fatalf("build(%q).Args = %v, want %v", tc.goos, cmd.Args, tc.wantArgs)
			}
			for i, a := range tc.wantArgs {
				if cmd.Args[i] != a {
					t.Fatalf("build(%q).Args = %v, want %v", tc.goos, cmd.Args, tc.wantArgs)
				}
			}
		})
	}

	if _, err := build("plan9", "/tmp/ep.mkv"); err == nil {
		t.Fatal("build(\"plan9\", ...) should return an error for unsupported OS")
	}
}
