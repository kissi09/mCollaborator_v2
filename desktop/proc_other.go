//go:build !windows

package main

import "os/exec"

// hideConsoleWindow is a Windows concern only; on macOS and Linux a child
// process started from a GUI app has no console to hide.
func hideConsoleWindow(cmd *exec.Cmd) {}
