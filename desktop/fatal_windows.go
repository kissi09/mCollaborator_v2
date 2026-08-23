//go:build windows

package main

import (
	"log"
	"os"
	"syscall"
	"unsafe"
)

// fatal reports a startup failure and exits.
//
// A Wails build is linked as a GUI binary, so it has no console and anything
// written to stderr goes nowhere the user will look. A message box is the only
// way to say why the app did not open.
func fatal(title string, err error) {
	log.Printf("fatal: %s: %v", title, err)

	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")

	const mbIconError = 0x00000010
	text, _ := syscall.UTF16PtrFromString(err.Error())
	caption, _ := syscall.UTF16PtrFromString(title)
	messageBox.Call(0,
		uintptr(unsafe.Pointer(text)),
		uintptr(unsafe.Pointer(caption)),
		mbIconError)

	os.Exit(1)
}
