package player

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Open launches filePath in the OS default application.
func Open(filePath string) error {
	cmd, err := build(runtime.GOOS, filePath)
	if err != nil {
		return err
	}
	return cmd.Start()
}

func build(goos, filePath string) (*exec.Cmd, error) {
	switch goos {
	case "linux":
		return exec.Command("xdg-open", filePath), nil
	case "darwin":
		return exec.Command("open", filePath), nil
	case "windows":
		return exec.Command("cmd", "/c", "start", "", filePath), nil
	default:
		return nil, fmt.Errorf("unsupported OS: %s", goos)
	}
}
