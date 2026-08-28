package player

import "testing"

func TestBuild(t *testing.T) {
	cases := []struct {
		goos     string
		wantArgs []string
	}{
		{"linux", []string{"xdg-open", "/tmp/ep.mkv"}},
		{"darwin", []string{"open", "/tmp/ep.mkv"}},
		{"windows", []string{"cmd", "/c", "start", "", "/tmp/ep.mkv"}},
	}

	for _, tc := range cases {
		t.Run(tc.goos, func(t *testing.T) {
			cmd, err := build(tc.goos, "/tmp/ep.mkv", "")
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

	if _, err := build("plan9", "/tmp/ep.mkv", ""); err == nil {
		t.Fatal("build(\"plan9\", ..., \"\") should return an error for unsupported OS")
	}
}

func TestBuild_CustomPlayerOverride(t *testing.T) {
	cmd, err := build("linux", "/tmp/ep.mkv", "vlc")
	if err != nil {
		t.Fatalf("build returned error: %v", err)
	}
	want := []string{"vlc", "/tmp/ep.mkv"}
	if len(cmd.Args) != len(want) || cmd.Args[0] != want[0] || cmd.Args[1] != want[1] {
		t.Fatalf("build with custom player = %v, want %v", cmd.Args, want)
	}
}

func TestIsMPV(t *testing.T) {
	cases := []struct {
		playerBin string
		want      bool
	}{
		{"", false},
		{"mpv", true},
		{"/usr/bin/mpv", true},
		{"vlc", false},
		{"mpv-wrapper", false},
	}
	for _, tc := range cases {
		if got := isMPV(tc.playerBin); got != tc.want {
			t.Errorf("isMPV(%q) = %v, want %v", tc.playerBin, got, tc.want)
		}
	}
}
