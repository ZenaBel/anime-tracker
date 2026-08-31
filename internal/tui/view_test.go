package tui

import "testing"

func TestScrollingText(t *testing.T) {
	s := "Hello World" // 11 runes

	if got := scrollingText(s, 20, true, 0); got != s {
		t.Errorf("fits within width: got %q, want %q", got, s)
	}
	if got := scrollingText(s, 5, false, 0); got != truncate(s, 5) {
		t.Errorf("unselected overflow: got %q, want truncate()'d %q", got, truncate(s, 5))
	}
	if got := scrollingText(s, 5, true, 0); got != "Hello" {
		t.Errorf("selected, tick 0: got %q, want %q (holding at start)", got, "Hello")
	}
	if got := scrollingText(s, 5, true, scrollHoldTicks-1); got != "Hello" {
		t.Errorf("selected, still within hold: got %q, want %q", got, "Hello")
	}
	if got := scrollingText(s, 5, true, scrollHoldTicks+1); got == "Hello" {
		t.Errorf("selected, past hold: expected the window to have advanced past %q, got %q", "Hello", got)
	}

	// full = "Hello World   " (14 runes); after one full lap (scrollHoldTicks
	// hold ticks + 14 scroll ticks) it must land back on the held start.
	full := len([]rune(s + "   "))
	if got := scrollingText(s, 5, true, scrollHoldTicks+full); got != "Hello" {
		t.Errorf("selected, one full lap later: got %q, want %q (wrapped back to start)", got, "Hello")
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
