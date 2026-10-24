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
	t.Setenv("DOT_HOME", home)
	t.Setenv("DOT_REPO", filepath.Join(root, "repo.git"))

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
		{"status", "extra"},
		{"p", "one", "two"}, // alias of push
	} {
		code, _, stderr := run(t, args...)

		if code != 2 || !strings.Contains(stderr, "usage: dot") {
			t.Errorf("%v: code=%d stderr=%q", args, code, stderr)
		}
	}
}

func TestCommandsNeedRepository(t *testing.T) {
	setup(t)

	code, _, stderr := run(t, "status")

	if code != 1 || !strings.Contains(stderr, "dot init") {
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

	config := filepath.Join(home, ".config", "app")
	if err := os.MkdirAll(config, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "conf"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	must("add", "~/.config/app")

	t.Chdir(filepath.Join(home, ".config"))

	if got := must("ls", "."); got != ".config/app/conf\n" {
		t.Fatalf("ls . = %q", got)
	}
	if got := must("list", "~"); got != ".config/app/conf\n.vimrc\n" {
		t.Fatalf("list ~ = %q", got)
	}
	if got := must("s"); got != "A  .config/app/conf\n M .vimrc\n" {
		t.Fatalf("s = %q", got)
	}

	must("rm", "~/.vimrc")

	if got := must("status"); got != "A  .config/app/conf\nD  .vimrc\n" {
		t.Fatalf("status after rm = %q", got)
	}
}

func TestVariants(t *testing.T) {
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

	rc := filepath.Join(home, ".rc")
	if err := os.WriteFile(rc, []byte("rc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	must("add", "~/.rc")

	// Limiting a plain file to this OS replaces it.
	got := must("add", "~/.rc", "--os")
	want := "Added .rc (os." + runtime.GOOS + "), which replaces the plain .rc\n"
	if got != want {
		t.Fatalf("add --os = %q, want %q", got, want)
	}

	// A variant for another class is stored, but not used here.
	if got := must("add", "--class=server", "~/.rc"); !strings.Contains(got, "does not use it") {
		t.Fatalf("add --class=server = %q", got)
	}

	if got := must("list"); got != ".rc (class.server) [not on this machine]\n.rc (os."+runtime.GOOS+")\n" {
		t.Fatalf("list = %q", got)
	}

	must("commit", "-m", "variants")

	// No ## files appear on disk.
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "##") {
			t.Fatalf("found %s on disk", entry.Name())
		}
	}

	// Edits go to the variant used here.
	if err := os.WriteFile(rc, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := must("status"); got != " M .rc (os."+runtime.GOOS+")\n" {
		t.Fatalf("status = %q", got)
	}
	must("add", "~/.rc")
	must("commit", "-m", "change")

	// With no variant for this OS, the plain file is the fallback.
	must("rm", "--os", "~/.rc")
	if got := must("add", "~/.rc"); got != "Added .rc\n" {
		t.Fatalf("add = %q", got)
	}
	must("commit", "-m", "fallback")

	// Switching the class switches the file, and back.
	for _, step := range []struct{ class, content string }{{"server", "rc\n"}, {"client", "changed\n"}} {
		class, content := step.class, step.content

		if got := must("config", "local.class", class); !strings.Contains(got, "wrote   .rc") {
			t.Fatalf("config local.class %s = %q", class, got)
		}
		if data, _ := os.ReadFile(rc); string(data) != content {
			t.Fatalf("class %s: .rc = %q", class, data)
		}
	}

	// A bare --host followed by a name is a likely mistake.
	if code, _, stderr := run(t, "add", "--host", "macbook", "~/.rc"); code != 2 || !strings.Contains(stderr, "--host=macbook") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}

	if code, _, _ := run(t, "add", "--os=bogus/x", "~/.rc"); code != 1 {
		t.Fatalf("bad condition should fail, code=%d", code)
	}
}
