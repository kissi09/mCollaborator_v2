//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// hideConsoleWindow stops the server child from flashing up a console window.
// The server is a console binary; without this, launching the desktop app pops
// a black window that then sits in the taskbar for the whole session.
func hideConsoleWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
