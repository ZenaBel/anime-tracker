package player

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"time"
)

const mpvSocketDialTimeout = 5 * time.Second

// openMPVWithIPC launches mpv with a JSON IPC socket and tracks playback
// completion over it. On Windows (where mpv's IPC uses named pipes rather
// than unix sockets, which this doesn't implement), it falls back to a
// plain fire-and-forget launch.
func openMPVWithIPC(mpvBin, filePath string) (<-chan PlaybackResult, error) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command(mpvBin, filePath)
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("starting mpv: %w", err)
		}
		return nil, nil
	}

	sockPath, err := mpvSocketPath()
	if err != nil {
		return nil, fmt.Errorf("preparing mpv ipc socket: %w", err)
	}

	cmd := exec.Command(mpvBin, "--input-ipc-server="+sockPath, filePath)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting mpv: %w", err)
	}

	resultCh := make(chan PlaybackResult, 1)
	go trackMPVPlayback(cmd, sockPath, resultCh)
	return resultCh, nil
}

func mpvSocketPath() (string, error) {
	f, err := os.CreateTemp("", "anime-tracker-mpv-*.sock")
	if err != nil {
		return "", err
	}
	path := f.Name()
	f.Close()
	os.Remove(path) // mpv creates the actual socket file at this path
	return path, nil
}

func trackMPVPlayback(cmd *exec.Cmd, sockPath string, resultCh chan<- PlaybackResult) {
	defer close(resultCh)

	watched := false
	if conn, err := dialMPVSocket(sockPath, mpvSocketDialTimeout); err == nil {
		watched = readUntilEndFile(conn)
		conn.Close()
	}

	// Report the result as soon as the outcome is known from the IPC
	// event, rather than waiting for the mpv process to fully exit
	// (which can lag behind end-file for window-close/cleanup reasons).
	// Reap the process and clean up the socket file in the background.
	resultCh <- PlaybackResult{Watched: watched}
	go func() {
		cmd.Wait()
		os.Remove(sockPath)
	}()
}

func dialMPVSocket(sockPath string, timeout time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", sockPath)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	return nil, fmt.Errorf("timed out connecting to mpv ipc socket: %w", lastErr)
}

// readUntilEndFile reads mpv's JSON IPC event stream until an "end-file"
// event arrives (or the connection closes), reporting whether playback
// reached EOF naturally (as opposed to being quit/stopped early).
func readUntilEndFile(conn net.Conn) bool {
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		var evt struct {
			Event  string `json:"event"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(sc.Bytes(), &evt); err != nil {
			continue
		}
		if evt.Event == "end-file" {
			return evt.Reason == "eof"
		}
	}
	return false
}
