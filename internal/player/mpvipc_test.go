package player

import (
	"net"
	"testing"
)

func TestReadUntilEndFile(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  bool
	}{
		{"eof", []string{`{"event":"start-file"}`, `{"event":"end-file","reason":"eof"}`}, true},
		{"quit", []string{`{"event":"end-file","reason":"quit"}`}, false},
		{"garbage-then-eof", []string{`not json`, `{"event":"end-file","reason":"eof"}`}, true},
		{"closed-without-event", []string{`{"event":"start-file"}`}, false},
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

			got := readUntilEndFile(client)
			if got != tc.want {
				t.Errorf("readUntilEndFile() = %v, want %v", got, tc.want)
			}
		})
	}
}
