package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	code := Run(args, &stdout, &stderr, BuildInfo{Version: "test"})

	return code, stdout.String(), stderr.String()
}

func setup(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	home := filepath.Join(root, "home")

	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("DOTMGR_HOME", home)
	t.Setenv("DOTMGR_REPO", filepath.Join(root, "repo.git"))

	return home
}

func TestPlatformCommand(t *testing.T) {
	code, stdout, stderr := run(t, "platform")

	if code != 0 || stdout == "" || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, stderr := run(t, "does-not-exist")

	if code != 2 || !strings.Contains(stderr, "unknown command") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	_, stdout, _ := run(t, "help")

	for _, c := range commands() {
		if !strings.Contains(stdout, c.name) {
			t.Errorf("help does not mention %q", c.name)
		}
	}
}

func TestUsageErrors(t *testing.T) {
	setup(t)

	for _, args := range [][]string{
		{"add"},
		{"rm"},
		{"commit"},
		{"commit", "-m", " "},
		{"clone"},
		{"remote"},
		{"remote", "add", "only-name"},
		{"list", "extra"},
	} {
		code, _, stderr := run(t, args...)

		if code != 2 || !strings.Contains(stderr, "usage: dotmgr") {
			t.Errorf("%v: code=%d stderr=%q", args, code, stderr)
		}
	}
}

func TestCommandsNeedRepository(t *testing.T) {
	setup(t)

	code, _, stderr := run(t, "status")

	if code != 1 || !strings.Contains(stderr, "dotmgr init") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestEndToEnd(t *testing.T) {
	home := setup(t)

	must := func(args ...string) string {
		t.Helper()

		code, stdout, stderr := run(t, args...)
		if code != 0 {
			t.Fatalf("%v: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}

		return stdout
	}

	must("init")
	must("config", "user.name", "Test")
	must("config", "user.email", "test@example.com")

	if got := strings.TrimSpace(must("config", "user.name")); got != "Test" {
		t.Fatalf("user.name = %q", got)
	}

	vimrc := filepath.Join(home, ".vimrc")
	if err := os.WriteFile(vimrc, []byte("set number\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	must("add", "~/.vimrc")

	if got := must("status"); got != "A  .vimrc\n" {
		t.Fatalf("status = %q", got)
	}

	if got := must("commit", "-m", "first"); !strings.Contains(got, "Committed") {
		t.Fatalf("commit output %q", got)
	}

	if got := must("commit", "-m", "again"); !strings.Contains(got, "Nothing to commit") {
		t.Fatalf("commit output %q", got)
	}

	if err := os.WriteFile(vimrc, []byte("set number\nset ruler\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := must("status"); got != " M .vimrc\n" {
		t.Fatalf("status = %q", got)
	}

	if got := must("diff"); !strings.Contains(got, "+set ruler") {
		t.Fatalf("diff = %q", got)
	}

	if got := must("list"); got != ".vimrc\n" {
		t.Fatalf("list = %q", got)
	}

	must("rm", "~/.vimrc")

	if got := must("status"); got != "D  .vimrc\n" {
		t.Fatalf("status after rm = %q", got)
	}
}
