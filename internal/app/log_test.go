package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// commitTwice creates a repository with two commits and returns the home
// directory.
func commitTwice(t *testing.T) {
	t.Helper()

	home := setup(t)

	for _, args := range [][]string{
		{"init"},
		{"config", "user.name", "Test"},
		{"config", "user.email", "test@example.org"},
	} {
		if code, _, stderr := run(t, args...); code != 0 {
			t.Fatalf("%v: %s", args, stderr)
		}
	}

	for index, message := range []string{"first", "second\n\nlonger text"} {
		write(t, home, ".rc", strings.Repeat("x", index+1))

		for _, args := range [][]string{{"add", "~/.rc"}, {"commit", "-m", message}} {
			if code, _, stderr := run(t, args...); code != 0 {
				t.Fatalf("%v: %s", args, stderr)
			}
		}
	}
}

func TestLogShowsNewestFirst(t *testing.T) {
	commitTwice(t)

	code, stdout, stderr := run(t, "log", "--no-pager")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}

	for _, want := range []string{"commit ", "Author: Test <test@example.org>", "Date:   ", "    second", "    longer text", "    first"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("log lacks %q:\n%s", want, stdout)
		}
	}

	if strings.Index(stdout, "second") > strings.Index(stdout, "first") {
		t.Errorf("newest commit must come first:\n%s", stdout)
	}

	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("output that is no terminal must not be colored:\n%s", stdout)
	}
}

func TestLogOnelineAndLimit(t *testing.T) {
	commitTwice(t)

	_, stdout, _ := run(t, "log", "--oneline")
	lines := strings.Split(strings.TrimSpace(stdout), "\n")

	if len(lines) != 2 || !strings.HasSuffix(lines[0], " second") || len(strings.Fields(lines[0])[0]) != 7 {
		t.Fatalf("oneline: %q", stdout)
	}

	for _, args := range [][]string{{"log", "--oneline", "-n", "1"}, {"log", "--oneline", "-1"}} {
		_, stdout, _ := run(t, args...)

		if strings.Count(stdout, "\n") != 1 {
			t.Errorf("%v: %q", args, stdout)
		}
	}
}

func TestLogColor(t *testing.T) {
	commitTwice(t)

	_, stdout, _ := run(t, "log", "--oneline", "--color=always")
	if !strings.Contains(stdout, colorYellow) {
		t.Errorf("--color=always: %q", stdout)
	}

	code, _, stderr := run(t, "log", "--color=sometimes")
	if code != 2 || !strings.Contains(stderr, "invalid value") {
		t.Errorf("code=%d stderr=%q", code, stderr)
	}
}

func TestLogWithoutCommits(t *testing.T) {
	setup(t)
	run(t, "init")

	code, stdout, _ := run(t, "log")
	if code != 0 || !strings.Contains(stdout, "No commits yet") {
		t.Fatalf("code=%d stdout=%q", code, stdout)
	}
}

func TestUseColor(t *testing.T) {
	var buffer bytes.Buffer // not a terminal

	for _, test := range []struct {
		when, configured string
		want, fails      bool
	}{
		{"always", "never", true, false}, // the flag wins
		{"never", "always", false, false},
		{"", "always", true, false},
		{"", "false", false, false},
		{"", "", false, false}, // auto, but no terminal
		{"", "auto", false, false},
		{"", "sometimes", false, true},
	} {
		got, err := useColor(&buffer, test.when, test.configured)

		if got != test.want || (err != nil) != test.fails {
			t.Errorf("%+v: got %v, %v", test, got, err)
		}
	}
}

func TestPagerCommand(t *testing.T) {
	t.Setenv("GIT_PAGER", "")
	t.Setenv("PAGER", "")

	// An empty GIT_PAGER switches the pager off, whatever else is set.
	if got := pagerCommand("most"); got != "" {
		t.Errorf("empty GIT_PAGER: %q", got)
	}

	t.Setenv("GIT_PAGER", "bat")
	if got := pagerCommand("most"); got != "bat" {
		t.Errorf("GIT_PAGER first: %q", got)
	}

	unset(t, "GIT_PAGER")
	if got := pagerCommand("most"); got != "most" {
		t.Errorf("core.pager second: %q", got)
	}

	t.Setenv("PAGER", "more")
	if got := pagerCommand(""); got != "more" {
		t.Errorf("PAGER third: %q", got)
	}

	if got := pagerCommand("cat"); got != "" {
		t.Errorf("cat means no pager: %q", got)
	}

	unset(t, "PAGER")
	if got := pagerCommand(""); got != "less" {
		t.Errorf("default: %q", got)
	}
}

func write(t *testing.T, home string, name string, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(home, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// unset removes an environment variable for the rest of the test.
func unset(t *testing.T, name string) {
	t.Helper()

	t.Setenv(name, "") // registers the restore of the old value
	os.Unsetenv(name)
}
