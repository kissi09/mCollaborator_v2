//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

// openWithDefaultApp hands the file to whatever is registered for its type -
// Word for a .docx, PowerPoint for a .pptx. ShellExecuteW is the call Explorer
// makes on a double click, so the association the user already has is the one
// that runs.
func openWithDefaultApp(path string) error {
	verb, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}

	shell32 := syscall.NewLazyDLL("shell32.dll")
	shellExecute := shell32.NewProc("ShellExecuteW")

	const swShowNormal = 1
	// ShellExecuteW is the one Win32 call that returns a success value greater
	// than 32; anything at or below that is an error code.
	ret, _, _ := shellExecute.Call(0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		0, 0, swShowNormal)
	if ret <= 32 {
		return fmt.Errorf("nothing on this machine opens %s (shell code %d)", path, ret)
	}
	return nil
}
