package player

import (
	"io"
	"net"
	"testing"
)

func TestReadPlaylistEvents(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		total int
		want  []PlaylistResult
	}{
		{
			name:  "all three finish naturally",
			total: 3,
			lines: []string{
				`{"event":"property-change","id":2,"name":"duration","data":600.0}`,
				`{"event":"property-change","id":1,"name":"time-pos","data":599.9}`,
				`{"event":"end-file","reason":"eof"}`,
				`{"event":"property-change","id":2,"name":"duration","data":700.0}`,
				`{"event":"property-change","id":1,"name":"time-pos","data":700.0}`,
				`{"event":"end-file","reason":"eof"}`,
				`{"event":"property-change","id":2,"name":"duration","data":500.0}`,
				`{"event":"property-change","id":1,"name":"time-pos","data":500.0}`,
				`{"event":"end-file","reason":"eof"}`,
			},
			want: []PlaylistResult{
				{FileIndex: 0, Watched: true, PositionSecs: 599.9, DurationSecs: 600.0},
				{FileIndex: 1, Watched: true, PositionSecs: 700.0, DurationSecs: 700.0},
				{FileIndex: 2, Watched: true, PositionSecs: 500.0, DurationSecs: 500.0},
			},
		},
		{
			name:  "quit partway through the second file",
			total: 3,
			lines: []string{
				`{"event":"property-change","id":2,"name":"duration","data":600.0}`,
				`{"event":"property-change","id":1,"name":"time-pos","data":600.0}`,
				`{"event":"end-file","reason":"eof"}`,
				`{"event":"property-change","id":2,"name":"duration","data":700.0}`,
				`{"event":"property-change","id":1,"name":"time-pos","data":250.5}`,
				`{"event":"end-file","reason":"quit"}`,
				// Never reaches the third file's events.
			},
			want: []PlaylistResult{
				{FileIndex: 0, Watched: true, PositionSecs: 600.0, DurationSecs: 600.0},
				{FileIndex: 1, Watched: false, PositionSecs: 250.5, DurationSecs: 700.0},
			},
		},
		{
			// Regression: manually clicking mpv's own "next" playlist
			// control ends the current file with reason "stop", not
			// "quit" — that must NOT be treated as the whole player
			// quitting, or the rest of the playlist stops being tracked
			// even though mpv keeps right on playing it.
			name:  "manual skip to next track mid-playlist, rest finish naturally",
			total: 3,
			lines: []string{
				`{"event":"property-change","id":2,"name":"duration","data":600.0}`,
				`{"event":"property-change","id":1,"name":"time-pos","data":45.0}`,
				`{"event":"end-file","reason":"stop","playlist_entry_id":1}`,
				`{"event":"start-file","playlist_entry_id":2}`,
				`{"event":"property-change","id":2,"name":"duration","data":700.0}`,
				`{"event":"property-change","id":1,"name":"time-pos","data":700.0}`,
				`{"event":"end-file","reason":"eof","playlist_entry_id":2}`,
				`{"event":"start-file","playlist_entry_id":3}`,
				`{"event":"property-change","id":2,"name":"duration","data":500.0}`,
				`{"event":"property-change","id":1,"name":"time-pos","data":500.0}`,
				`{"event":"end-file","reason":"eof","playlist_entry_id":3}`,
			},
			want: []PlaylistResult{
				{FileIndex: 0, Watched: false, PositionSecs: 45.0, DurationSecs: 600.0},
				{FileIndex: 1, Watched: true, PositionSecs: 700.0, DurationSecs: 700.0},
				{FileIndex: 2, Watched: true, PositionSecs: 500.0, DurationSecs: 500.0},
			},
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
			go io.Copy(io.Discard, server)

			resultCh := make(chan PlaylistResult, tc.total)
			readPlaylistEvents(client, tc.total, resultCh)
			close(resultCh)

			var got []PlaylistResult
			for r := range resultCh {
				got = append(got, r)
			}

			if len(got) != len(tc.want) {
				t.Fatalf("got %d results, want %d: %+v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("result[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}
