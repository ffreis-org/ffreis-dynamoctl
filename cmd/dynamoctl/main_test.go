package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestMainRunsVersion(t *testing.T) {
	origArgs := os.Args
	origStdout := os.Stdout
	origStderr := os.Stderr
	origExit := exit
	defer func() {
		os.Args = origArgs
		os.Stdout = origStdout
		os.Stderr = origStderr
		exit = origExit
	}()

	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe stdout: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe stderr: %v", err)
	}

	os.Stdout = wOut
	os.Stderr = wErr
	os.Args = []string{"dynamoctl", "version"}
	exit = func(int) {
		// Stubbed to prevent os.Exit from terminating the test process.
	}

	main()

	_ = wOut.Close()
	_ = wErr.Close()

	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, rOut)
	_ = rOut.Close()

	var stderr bytes.Buffer
	_, _ = io.Copy(&stderr, rErr)
	_ = rErr.Close()

	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "commit=") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestMainExitsNonZeroOnError(t *testing.T) {
	origArgs := os.Args
	origStdout := os.Stdout
	origStderr := os.Stderr
	origExit := exit
	defer func() {
		os.Args = origArgs
		os.Stdout = origStdout
		os.Stderr = origStderr
		exit = origExit
	}()

	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe stdout: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe stderr: %v", err)
	}

	os.Stdout = wOut
	os.Stderr = wErr
	os.Args = []string{"dynamoctl", "definitely-not-a-command"}

	gotExit := 0
	exit = func(code int) { gotExit = code }

	main()

	_ = wOut.Close()
	_ = wErr.Close()

	var stdout bytes.Buffer
	_, _ = io.Copy(&stdout, rOut)
	_ = rOut.Close()

	var stderr bytes.Buffer
	_, _ = io.Copy(&stderr, rErr)
	_ = rErr.Close()

	if gotExit != 1 {
		t.Fatalf("exit code: want 1, got %d", gotExit)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}
