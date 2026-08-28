package tui

import "testing"

func TestPercentBar(t *testing.T) {
	cases := []struct {
		percent int
		width   int
		want    string
	}{
		{0, 10, "[----------]"},
		{100, 10, "[##########]"},
		{50, 10, "[#####-----]"},
		{-5, 10, "[----------]"},
		{150, 10, "[##########]"},
		{25, 4, "[#---]"},
	}
	for _, tc := range cases {
		if got := percentBar(tc.percent, tc.width); got != tc.want {
			t.Errorf("percentBar(%d, %d) = %q, want %q", tc.percent, tc.width, got, tc.want)
		}
	}
}
