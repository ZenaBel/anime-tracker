package tui

import "testing"

func TestScrollingText(t *testing.T) {
	s := "Hello World" // 11 runes, extra = 11-5 = 6 positions to reveal the tail

	if got := scrollingText(s, 20, true, 0); got != s {
		t.Errorf("fits within width: got %q, want %q", got, s)
	}
	if got := scrollingText(s, 5, false, 0); got != truncate(s, 5) {
		t.Errorf("unselected overflow: got %q, want truncate()'d %q", got, truncate(s, 5))
	}

	// Holds at the start for scrollHoldTicks.
	if got := scrollingText(s, 5, true, 0); got != "Hello" {
		t.Errorf("selected, tick 0: got %q, want %q (holding at start)", got, "Hello")
	}
	if got := scrollingText(s, 5, true, scrollHoldTicks-1); got != "Hello" {
		t.Errorf("selected, still within start hold: got %q, want %q", got, "Hello")
	}

	// Slides forward to fully reveal the tail, and holds there too — every
	// frame along the way must be a real contiguous substring of s.
	if got := scrollingText(s, 5, true, scrollHoldTicks+6); got != "World" {
		t.Errorf("selected, tail fully revealed: got %q, want %q", got, "World")
	}
	if got := scrollingText(s, 5, true, 2*scrollHoldTicks+6-1); got != "World" {
		t.Errorf("selected, still within tail hold: got %q, want %q", got, "World")
	}

	// Slides back and, after one full lap, lands on the held start again.
	cycle := 2*scrollHoldTicks + 2*6
	if got := scrollingText(s, 5, true, cycle); got != "Hello" {
		t.Errorf("selected, one full lap later: got %q, want %q (back at start)", got, "Hello")
	}
}

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
