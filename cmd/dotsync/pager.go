package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"os/signal"
)

// stdoutIsTTY is a test seam.
var stdoutIsTTY = func() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// pagerCommand returns the pager shell command, or "" when paging is disabled.
func pagerCommand() string {
	p := "less"
	if v := os.Getenv("DOTSYNC_PAGER"); v != "" {
		p = v
	} else if v := os.Getenv("PAGER"); v != "" {
		p = v
	}
	switch p {
	case "cat":
		return ""
	case "less":
		// -F quits when output fits one screen, overriding e.g. LESS=-R.
		return "less -FRX"
	}
	return p
}

// page pipes out through the pager, falling back to plain stdout if it cannot run.
func page(out []byte) {
	cmd := exec.Command("sh", "-c", pagerCommand())
	cmd.Stdin = bytes.NewReader(out)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Ctrl-C in less must not kill dotsync and leave the terminal broken.
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)
	err := cmd.Run()
	var ee *exec.ExitError
	if err != nil && (!errors.As(err, &ee) || ee.ExitCode() == 127) {
		os.Stdout.Write(out)
	}
}
