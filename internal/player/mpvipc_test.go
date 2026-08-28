package player

import (
	"io"
	"net"
	"testing"
)

func TestReadPlaybackEvents(t *testing.T) {
	cases := []struct {
		name         string
		lines        []string
		wantWatched  bool
		wantPosition float64
		wantDuration float64
	}{
		{
			name:        "eof",
			lines:       []string{`{"event":"start-file"}`, `{"event":"end-file","reason":"eof"}`},
			wantWatched: true,
		},
		{
			name:        "quit",
			lines:       []string{`{"event":"end-file","reason":"quit"}`},
			wantWatched: false,
		},
		{
			name:        "garbage-then-eof",
			lines:       []string{`not json`, `{"event":"end-file","reason":"eof"}`},
			wantWatched: true,
		},
		{
			name:        "closed-without-event",
			lines:       []string{`{"event":"start-file"}`},
			wantWatched: false,
		},
		{
			name: "tracks position and duration then quits early",
			lines: []string{
				`{"event":"property-change","id":2,"name":"duration","data":1420.5}`,
				`{"event":"property-change","id":1,"name":"time-pos","data":300.25}`,
				`{"event":"property-change","id":1,"name":"time-pos","data":612.0}`,
				`{"event":"end-file","reason":"quit"}`,
			},
			wantWatched:  false,
			wantPosition: 612.0,
			wantDuration: 1420.5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, client := net.Pipe()
			go func() {
				for _, l := range tc.lines {
					server.Write([]byte(l + "\n"))
				}
				server.Close()
			}()
			// Drain the observe_property commands readPlaybackEvents
			// writes to the client end, so those writes don't block.
			go io.Copy(io.Discard, server)

			got := readPlaybackEvents(client)
			if got.Watched != tc.wantWatched {
				t.Errorf("Watched = %v, want %v", got.Watched, tc.wantWatched)
			}
			if got.PositionSecs != tc.wantPosition {
				t.Errorf("PositionSecs = %v, want %v", got.PositionSecs, tc.wantPosition)
			}
			if got.DurationSecs != tc.wantDuration {
				t.Errorf("DurationSecs = %v, want %v", got.DurationSecs, tc.wantDuration)
			}
		})
	}
}
