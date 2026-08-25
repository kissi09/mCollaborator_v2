//go:build !windows

package main

import (
	"os/exec"
	"runtime"
)

// openWithDefaultApp is the macOS and Linux side of the same job. Neither
// installer is built yet, but the shell compiles for both and this is the only
// platform-specific thing the report handoff needs.
func openWithDefaultApp(path string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, path).Start()
}
