//go:build !windows

package main

import (
	"fmt"
	"log"
	"os"
)

// fatal reports a startup failure and exits. On macOS and Linux the message
// goes to stderr, which is where a desktop launcher's own logging picks it up.
func fatal(title string, err error) {
	log.Printf("fatal: %s: %v", title, err)
	fmt.Fprintf(os.Stderr, "%s: %v\n", title, err)
	os.Exit(1)
}
