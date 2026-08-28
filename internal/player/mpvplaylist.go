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
// "end-file" event as it arrives. A file's own end-file reason is either
// "eof" (played through) or something else — "stop" in particular fires
// just as much for the user manually skipping to the next/previous track
// in mpv's own playlist controls as for an actual full quit, so a non-eof
// reason on its own does NOT mean tracking should stop: only an explicit
// "quit" reason (the whole player exiting) or having accounted for every
// file does. File identity comes from mpv's own playlist_entry_id (1-based,
// stable even if the user jumps around the playlist out of order) with a
// sequential counter as a fallback for mpv versions that omit it.
func readPlaylistEvents(conn net.Conn, total int, resultCh chan<- PlaylistResult) {
	enc := json.NewEncoder(conn)
	enc.Encode(map[string]any{"command": []any{"observe_property", 1, "time-pos"}})
	enc.Encode(map[string]any{"command": []any{"observe_property", 2, "duration"}})

	var pos, dur float64
	seen := make(map[int]bool)
	reported := 0
	fallbackIdx := 0
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		var msg struct {
			Event           string          `json:"event"`
			Reason          string          `json:"reason"`
			Name            string          `json:"name"`
			Data            json.RawMessage `json:"data"`
			PlaylistEntryID int             `json:"playlist_entry_id"`
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

		if msg.Event != "end-file" {
			continue
		}

		fileIdx := fallbackIdx
		if msg.PlaylistEntryID >= 1 {
			fileIdx = msg.PlaylistEntryID - 1
		}

		if fileIdx >= 0 && fileIdx < total && !seen[fileIdx] {
			resultCh <- PlaylistResult{FileIndex: fileIdx, Watched: msg.Reason == "eof", PositionSecs: pos, DurationSecs: dur}
			seen[fileIdx] = true
			reported++
			fallbackIdx = fileIdx + 1
		}
		pos, dur = 0, 0 // whatever plays next starts its own position tracking

		if msg.Reason == "quit" || reported >= total {
			return
		}
	}
}
