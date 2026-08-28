package player

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
)

// openMPVPlaylistWithIPC launches mpv with filePaths as a sequential
// playlist and tracks per-file completion over its JSON IPC socket. On
// Windows it falls back to a plain fire-and-forget launch, same as
// openMPVWithIPC.
func openMPVPlaylistWithIPC(mpvBin string, filePaths []string) (<-chan PlaylistResult, error) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command(mpvBin, filePaths...)
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("starting mpv: %w", err)
		}
		return nil, nil
	}

	sockPath, err := mpvSocketPath()
	if err != nil {
		return nil, fmt.Errorf("preparing mpv ipc socket: %w", err)
	}

	args := append([]string{"--input-ipc-server=" + sockPath}, filePaths...)
	cmd := exec.Command(mpvBin, args...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting mpv: %w", err)
	}

	// Buffered so readPlaylistEvents never blocks mid-scan waiting for a
	// slow consumer to drain a result before it can read the next event.
	resultCh := make(chan PlaylistResult, len(filePaths))
	go trackMPVPlaylist(cmd, sockPath, len(filePaths), resultCh)
	return resultCh, nil
}

func trackMPVPlaylist(cmd *exec.Cmd, sockPath string, total int, resultCh chan<- PlaylistResult) {
	defer close(resultCh)

	if conn, err := dialMPVSocket(sockPath, mpvSocketDialTimeout); err == nil {
		readPlaylistEvents(conn, total, resultCh)
		conn.Close()
	}

	go func() {
		cmd.Wait()
		os.Remove(sockPath)
	}()
}

// readPlaylistEvents subscribes to mpv's time-pos and duration properties
// and reads its JSON IPC event stream, emitting one PlaylistResult per
// "end-file" event in the order they arrive (which matches playlist
// order, since mpv plays queued files sequentially). Stops once every
// file has reported a result, or on the first non-eof end-file (an early
// quit/stop skips whatever files hadn't started yet).
func readPlaylistEvents(conn net.Conn, total int, resultCh chan<- PlaylistResult) {
	enc := json.NewEncoder(conn)
	enc.Encode(map[string]any{"command": []any{"observe_property", 1, "time-pos"}})
	enc.Encode(map[string]any{"command": []any{"observe_property", 2, "duration"}})

	var pos, dur float64
	fileIdx := 0
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		var msg struct {
			Event  string          `json:"event"`
			Reason string          `json:"reason"`
			Name   string          `json:"name"`
			Data   json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			continue
		}

		if msg.Event == "property-change" {
			var v float64
			if err := json.Unmarshal(msg.Data, &v); err == nil {
				switch msg.Name {
				case "time-pos":
					pos = v
				case "duration":
					dur = v
				}
			}
			continue
		}

		if msg.Event != "end-file" || fileIdx >= total {
			continue
		}

		watched := msg.Reason == "eof"
		resultCh <- PlaylistResult{FileIndex: fileIdx, Watched: watched, PositionSecs: pos, DurationSecs: dur}
		fileIdx++
		pos, dur = 0, 0 // next file starts its own position tracking

		if !watched || fileIdx >= total {
			return
		}
	}
}
