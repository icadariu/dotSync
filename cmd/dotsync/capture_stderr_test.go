package main

import (
	"io"
	"os"
	"strings"
)

// captureStderr returns what f writes to os.Stderr.
func captureStderr(f func()) string {
	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w
	f()
	w.Close()
	os.Stderr = old
	var sb strings.Builder
	_, _ = io.Copy(&sb, r)
	return sb.String()
}
