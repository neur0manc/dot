package store

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func skipWithoutSymlinks(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
}

// setupAlternates tracks .cfg##... files and returns the machine.
func setupAlternates(t *testing.T) (machine, *Store) {
	t.Helper()

	m := newMachine(t)
	s := m.init(t)

	m.write(t, ".cfg##default", "default\n")
	m.write(t, ".cfg##os.Linux", "linux\n")
	m.write(t, ".cfg##os.Darwin", "darwin\n")
	m.write(t, ".cfg##hostname.work,os.Linux", "work linux\n")
	m.write(t, ".cfg##class.server", "server\n")
	m.write(t, ".plain", "plain\n")

	if _, err := s.Add("~/.cfg##default", "~/.cfg##os.Linux", "~/.cfg##os.Darwin",
		"~/.cfg##hostname.work,os.Linux", "~/.cfg##class.server", "~/.plain"); err != nil {
		t.Fatal(err)
	}

	return m, s
}

func TestAltPicksTheMostSpecificMatch(t *testing.T) {
	skipWithoutSymlinks(t)

	cases := []struct {
		name  string
		facts Facts
		want  string
	}{
		{"default", Facts{OS: "freebsd"}, "default\n"},
		{"os", Facts{OS: "linux"}, "linux\n"},
		{"os alias and case", Facts{OS: "Darwin"}, "darwin\n"},
		{"hostname and os", Facts{OS: "linux", Hostname: "WORK"}, "work linux\n"},
		{"all conditions must match", Facts{OS: "darwin", Hostname: "work"}, "darwin\n"},
		{"class", Facts{OS: "freebsd", Class: "server"}, "server\n"},
		{"os beats class", Facts{OS: "linux", Class: "server"}, "linux\n"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			m, s := setupAlternates(t)

			if _, err := s.Alt(test.facts, false); err != nil {
				t.Fatal(err)
			}

			if got := m.read(t, ".cfg"); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}

			target, err := os.Readlink(filepath.Join(m.home, ".cfg"))
			if err != nil || !strings.HasPrefix(target, ".cfg##") {
				t.Fatalf("expected a relative link, got %q %v", target, err)
			}
		})
	}
}

func TestAltUpdatesAndRemovesGeneratedFiles(t *testing.T) {
	skipWithoutSymlinks(t)

	m, s := setupAlternates(t)

	result, err := s.Alt(Facts{OS: "linux"}, false)
	if err != nil || len(result.Created) != 1 {
		t.Fatalf("first run: %+v %v", result, err)
	}

	result, err = s.Alt(Facts{OS: "linux"}, false)
	if err != nil || len(result.Created) != 0 || len(result.Unchanged) != 1 {
		t.Fatalf("second run should change nothing: %+v %v", result, err)
	}

	result, err = s.Alt(Facts{OS: "darwin"}, false)
	if err != nil || len(result.Created) != 1 || m.read(t, ".cfg") != "darwin\n" {
		t.Fatalf("switching system: %+v %v", result, err)
	}

	// Without a default and without a match the generated file disappears.
	if _, err := s.Remove("~/.cfg##default"); err != nil {
		t.Fatal(err)
	}

	result, err = s.Alt(Facts{OS: "freebsd"}, false)
	if err != nil || len(result.Removed) != 1 || m.exists(".cfg") {
		t.Fatalf("stale file not removed: %+v %v", result, err)
	}

	if !m.exists(".cfg##os.Linux") {
		t.Fatal("alternates themselves must stay")
	}
}

func TestAltDoesNotOverwriteForeignFiles(t *testing.T) {
	skipWithoutSymlinks(t)

	m, s := setupAlternates(t)
	m.write(t, ".cfg", "mine\n")

	result, err := s.Alt(Facts{OS: "linux"}, false)
	if err != nil || len(result.Conflicts) != 1 || m.read(t, ".cfg") != "mine\n" {
		t.Fatalf("conflict expected: %+v %v", result, err)
	}

	result, err = s.Alt(Facts{OS: "linux"}, true)
	if err != nil || len(result.Created) != 1 || m.read(t, ".cfg") != "linux\n" {
		t.Fatalf("force should replace: %+v %v", result, err)
	}

	// A file that already has the right content is adopted silently.
	if err := os.Remove(filepath.Join(m.home, ".cfg")); err != nil {
		t.Fatal(err)
	}
	m.write(t, ".cfg", "linux\n")

	result, err = s.Alt(Facts{OS: "linux", UseCopies: true}, false)
	if err != nil || len(result.Conflicts) != 0 {
		t.Fatalf("identical file should not conflict: %+v %v", result, err)
	}
}

func TestAltCopiesAndNoticesEditedCopies(t *testing.T) {
	m, s := setupAlternates(t)

	facts := Facts{OS: "linux", UseCopies: true}

	result, err := s.Alt(facts, false)
	if err != nil || len(result.Created) != 1 || !result.Created[0].Copy {
		t.Fatalf("copy expected: %+v %v", result, err)
	}

	if info, err := os.Lstat(filepath.Join(m.home, ".cfg")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("expected a regular file: %v %v", info, err)
	}

	// The alternate changes: an untouched copy follows it.
	m.write(t, ".cfg##os.Linux", "linux v2\n")

	if _, err := s.Alt(facts, false); err != nil || m.read(t, ".cfg") != "linux v2\n" {
		t.Fatalf("copy not refreshed: %v", err)
	}

	// A copy the user edited is not replaced.
	m.write(t, ".cfg", "edited\n")
	m.write(t, ".cfg##os.Linux", "linux v3\n")

	result, err = s.Alt(facts, false)
	if err != nil || len(result.Conflicts) != 1 || m.read(t, ".cfg") != "edited\n" {
		t.Fatalf("edited copy must be kept: %+v %v", result, err)
	}
}

func TestAddSkipsGeneratedFiles(t *testing.T) {
	skipWithoutSymlinks(t)

	m, s := setupAlternates(t)

	if _, err := s.Alt(Facts{OS: "linux"}, false); err != nil {
		t.Fatal(err)
	}

	m.write(t, ".config/app/b", "b\n")

	added, err := s.Add("~")
	if err == nil {
		t.Fatalf("home itself must be rejected, added %v", added)
	}

	if _, err := s.Add("~/.cfg"); err == nil {
		t.Fatal("a generated file must not be added directly")
	}

	m.write(t, "dir/x##os.Linux", "x\n")
	m.write(t, "dir/other", "o\n")

	if _, err := s.Alt(Facts{OS: "linux"}, false); err != nil {
		t.Fatal(err)
	}

	added, err = s.Add("~/dir")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Join(added, ",") != "dir/other,dir/x##os.Linux" {
		t.Fatalf("added %v", added)
	}
}

func TestClassConfig(t *testing.T) {
	m := newMachine(t)
	s := m.init(t)

	if err := s.SetUserConfig(ClassConfigKey, "laptop"); err != nil {
		t.Fatal(err)
	}

	got, err := s.UserConfig(ClassConfigKey)
	if err != nil || got != "laptop" {
		t.Fatalf("class = %q %v", got, err)
	}

	facts, err := s.SystemFacts()
	if err != nil || facts.Class != "laptop" || facts.OS != runtime.GOOS {
		t.Fatalf("facts = %+v %v", facts, err)
	}

	// Survives reopening.
	reopened, err := Open(m.gitDir, m.home)
	if err != nil {
		t.Fatal(err)
	}

	if got, _ := reopened.UserConfig(ClassConfigKey); got != "laptop" {
		t.Fatalf("class after reopen = %q", got)
	}
}

func TestParseAlternate(t *testing.T) {
	cases := map[string]struct {
		base string
		ok   bool
	}{
		".cfg##os.Linux":         {".cfg", true},
		"a/b/c##default":         {"a/b/c", true},
		"dir##x/file":            {"", false}, // separator in a directory only
		"##os.Linux":             {"", false},
		"name##":                 {"", false},
		"plain":                  {"", false},
		"a##hostname.h,os.Linux": {"a", true},
	}

	for name, want := range cases {
		base, _, ok := parseAlternate(name)
		if ok != want.ok || base != want.base {
			t.Errorf("parseAlternate(%q) = %q %v, want %q %v", name, base, ok, want.base, want.ok)
		}
	}
}

func TestAddAlternateRenamesAndLinksBack(t *testing.T) {
	skipWithoutSymlinks(t)

	m := newMachine(t)
	s := m.init(t)

	m.write(t, ".ssh/config", "Host *\n")
	if err := os.Chmod(filepath.Join(m.home, ".ssh", "config"), 0o600); err != nil {
		t.Fatal(err)
	}

	facts, err := s.SystemFacts()
	if err != nil {
		t.Fatal(err)
	}

	conditions := "os." + facts.OS + ",hostname." + facts.Hostname

	result, err := s.AddAlternate(conditions, "~/.ssh/config")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Moved) != 1 || len(result.Copied) != 0 ||
		result.Added[0] != ".ssh/config##"+conditions {
		t.Fatalf("unexpected result %+v", result)
	}

	// The original is gone until Alt links it back.
	if m.exists(".ssh/config") {
		t.Fatal("original should have been renamed")
	}

	if _, err := s.Alt(facts, false); err != nil {
		t.Fatal(err)
	}

	if m.read(t, ".ssh/config") != "Host *\n" {
		t.Fatal("content must be reachable under the original name")
	}

	info, err := os.Stat(filepath.Join(m.home, ".ssh", "config##"+conditions))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions of the alternate: %v %v", info, err)
	}

	expectStatus(t, s, "A  .ssh/config##"+conditions)
}

func TestAddAlternateKeepsFileThatDoesNotApplyHere(t *testing.T) {
	m := newMachine(t)
	s := m.init(t)

	m.write(t, ".rc", "x\n")

	result, err := s.AddAlternate("##os.plan9", "~/.rc")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Copied) != 1 || len(result.Moved) != 0 {
		t.Fatalf("unexpected result %+v", result)
	}

	if m.read(t, ".rc") != "x\n" || m.read(t, ".rc##os.plan9") != "x\n" {
		t.Fatal("both the original and the alternate must exist")
	}
}

func TestAddAlternateRejectsBadInput(t *testing.T) {
	skipWithoutSymlinks(t)

	m, s := setupAlternates(t)

	for _, test := range []struct {
		conditions string
		path       string
	}{
		{"", "~/.plain"},
		{"planet.mars", "~/.plain"},
		{"os.", "~/.plain"},
		{"os.linux", "~/.missing"},
		{"os.linux", "~/.cfg##default"}, // already an alternate
	} {
		if _, err := s.AddAlternate(test.conditions, test.path); err == nil {
			t.Errorf("AddAlternate(%q, %q) should fail", test.conditions, test.path)
		}
	}

	// A generated file cannot be turned into an alternate.
	if _, err := s.Alt(Facts{OS: "linux"}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := s.AddAlternate("os.linux", "~/.cfg"); err == nil {
		t.Error("a generated file must be rejected")
	}

	// An existing target is never overwritten.
	m.write(t, ".plain##os.linux", "other\n")

	if _, err := s.AddAlternate("os.linux", "~/.plain"); err == nil {
		t.Error("existing alternate must not be overwritten")
	}

	if m.read(t, ".plain") != "plain\n" {
		t.Error("original must be untouched after a failure")
	}
}
