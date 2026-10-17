package app

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
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

func TestAltCommand(t *testing.T) {
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
	must("config", "local.class", "work")

	if got := strings.TrimSpace(must("config", "local.class")); got != "work" {
		t.Fatalf("class = %q", got)
	}

	for name, content := range map[string]string{
		".gitconfig##default":    "personal\n",
		".gitconfig##class.work": "work\n",
	} {
		if err := os.WriteFile(filepath.Join(home, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	must("add", "~/.gitconfig##default", "~/.gitconfig##class.work")

	if got := must("alt"); !strings.Contains(got, ".gitconfig <- .gitconfig##class.work") {
		t.Fatalf("alt output = %q", got)
	}

	data, err := os.ReadFile(filepath.Join(home, ".gitconfig"))
	if err != nil || string(data) != "work\n" {
		t.Fatalf(".gitconfig = %q %v", data, err)
	}

	// The generated file does not show up as a change.
	if got := must("status"); !strings.HasPrefix(got, "A  .gitconfig##") {
		t.Fatalf("status = %q", got)
	}

	// A foreign file is reported, not overwritten.
	must("config", "local.class", "other")

	if err := os.Remove(filepath.Join(home, ".gitconfig")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := run(t, "alt")
	if code != 1 || !strings.Contains(stderr, "alt --force") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestAddAltFlag(t *testing.T) {
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

	if err := os.WriteFile(filepath.Join(home, ".rc"), []byte("rc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	hostname, _ := os.Hostname()
	hostname, _, _ = strings.Cut(hostname, ".")

	for _, form := range [][]string{
		{"add", "--alt", "hostname." + hostname, "~/.rc"},
	} {
		got := must(form...)

		if !strings.Contains(got, "Added .rc##hostname.") ||
			(runtime.GOOS != "windows" && !strings.Contains(got, "linked .rc")) {
			t.Fatalf("output = %q", got)
		}
	}

	if data, err := os.ReadFile(filepath.Join(home, ".rc")); err != nil || string(data) != "rc\n" {
		t.Fatalf(".rc = %q %v", data, err)
	}

	if code, _, stderr := run(t, "add", "--alt"); code != 2 {
		t.Fatalf("missing value: code=%d stderr=%q", code, stderr)
	}

	if code, _, _ := run(t, "add", "--alt", "bogus", "~/.rc"); code != 1 {
		t.Fatalf("bad condition should fail, code=%d", code)
	}
}
