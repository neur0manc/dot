package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlatformCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"platform"},
		&stdout,
		&stderr,
		BuildInfo{},
	)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if stdout.Len() == 0 {
		t.Fatal("expected platform output")
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected no error output, got %q", stderr.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"does-not-exist"},
		&stdout,
		&stderr,
		BuildInfo{},
	)

	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf(
			"expected unknown-command error, got %q",
			stderr.String(),
		)
	}
}
