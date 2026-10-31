package store

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// machine is one simulated computer: a home directory and a Git directory.
type machine struct {
	home   string
	gitDir string
}

func newMachine(t *testing.T) machine {
	t.Helper()

	root := t.TempDir()
	home := filepath.Join(root, "home")

	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}

	// Keep the developer's real ~/.gitconfig out of the tests.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	return machine{home: home, gitDir: filepath.Join(root, "repo.git")}
}

func (m machine) init(t *testing.T) *Store {
	t.Helper()

	s, err := Init(m.gitDir, m.home)
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := s.SetConfig("user.name", "Test"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetConfig("user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}

	return s
}

func (m machine) write(t *testing.T, name string, content string) {
	t.Helper()

	filename := filepath.Join(m.home, filepath.FromSlash(name))

	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (m machine) read(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(m.home, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

func (m machine) exists(name string) bool {
	_, err := os.Lstat(filepath.Join(m.home, filepath.FromSlash(name)))
	return err == nil
}

func addedNames(added []AddedFile) []string {
	names := make([]string, 0, len(added))
	for _, file := range added {
		names = append(names, file.Name)
	}

	return names
}

func mustCommit(t *testing.T, s *Store, message string) {
	t.Helper()

	if _, err := s.Commit(message); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func statusLines(t *testing.T, s *Store) []string {
	t.Helper()

	states, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}

	lines := []string{}
	for _, state := range states {
		lines = append(lines, state.String())
	}

	return lines
}

func expectStatus(t *testing.T, s *Store, want ...string) {
	t.Helper()

	got := strings.Join(statusLines(t, s), "|")
	if got != strings.Join(want, "|") {
		t.Fatalf("status = %q, want %q", got, strings.Join(want, "|"))
	}
}

func TestInitDoesNotTouchHome(t *testing.T) {
	m := newMachine(t)
	m.init(t)

	entries, err := os.ReadDir(m.home)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 0 {
		t.Fatalf("home should stay empty, found %v", entries)
	}

	if _, err := Init(m.gitDir, m.home); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("second init: want ErrAlreadyExists, got %v", err)
	}
}

func TestOpenWithoutRepository(t *testing.T) {
	m := newMachine(t)

	if _, err := Open(m.gitDir, m.home); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("want ErrNotInitialized, got %v", err)
	}
}

func TestAddStatusCommitCycle(t *testing.T) {
	m := newMachine(t)
	s := m.init(t)

	m.write(t, ".vimrc", "set number\n")
	m.write(t, ".config/app/a.conf", "a\n")
	m.write(t, ".config/app/b.conf", "b\n")
	m.write(t, "untracked.txt", "ignored\n")

	added, err := s.Add("~/.vimrc", filepath.Join(m.home, ".config", "app"))
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	want := ".config/app/a.conf,.config/app/b.conf,.vimrc"
	if strings.Join(addedNames(added), ",") != want {
		t.Fatalf("added %v, want %s", added, want)
	}

	expectStatus(t, s,
		"A  .config/app/a.conf",
		"A  .config/app/b.conf",
		"A  .vimrc",
	)

	mustCommit(t, s, "initial")
	expectStatus(t, s)

	m.write(t, ".vimrc", "set number\nset ruler\n")
	expectStatus(t, s, " M .vimrc")

	diffs, err := s.Diff(false)
	if err != nil || len(diffs) != 1 {
		t.Fatalf("diff: %v %v", diffs, err)
	}

	if !strings.Contains(diffs[0].Patch, "+set ruler") {
		t.Fatalf("unexpected patch:\n%s", diffs[0].Patch)
	}

	if _, err := s.Add(".vimrc"); err == nil {
		// A relative path resolves against the working directory, which is
		// not the home directory, so this must be rejected.
		t.Fatal("expected relative path outside home to be rejected")
	}

	if _, err := s.Add("~/.vimrc"); err != nil {
		t.Fatal(err)
	}

	expectStatus(t, s, "M  .vimrc")

	staged, err := s.Diff(true)
	if err != nil || len(staged) != 1 {
		t.Fatalf("staged diff: %v %v", staged, err)
	}

	mustCommit(t, s, "ruler")

	if err := os.Remove(filepath.Join(m.home, ".vimrc")); err != nil {
		t.Fatal(err)
	}

	expectStatus(t, s, " D .vimrc")

	if _, err := s.Commit("nothing"); !errors.Is(err, ErrNoStagedChanges) {
		t.Fatalf("want ErrNoStagedChanges, got %v", err)
	}
}

func TestAddRejectsOutsideHomeAndRepository(t *testing.T) {
	m := newMachine(t)
	s := m.init(t)

	outside := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Add(outside); err == nil {
		t.Fatal("expected file outside home to be rejected")
	}

	if _, err := s.Add("~"); err == nil {
		t.Fatal("expected the home directory itself to be rejected")
	}

	if _, err := s.Add("~/missing"); err == nil {
		t.Fatal("expected missing file to be rejected")
	}
}

func TestAddSkipsRepositoryInsideHome(t *testing.T) {
	m := newMachine(t)
	m.gitDir = filepath.Join(m.home, ".local", "share", "dot", "repo.git")
	s := m.init(t)

	m.write(t, ".local/share/dot/other", "x\n")
	m.write(t, ".local/share/keep", "k\n")

	added, err := s.Add("~/.local")
	if err != nil {
		t.Fatal(err)
	}

	got := strings.Join(addedNames(added), ",")
	if got != ".local/share/dot/other,.local/share/keep" {
		t.Fatalf("added %s", got)
	}

	if _, err := s.Add(m.gitDir); err == nil {
		t.Fatal("expected repository directory to be rejected")
	}
}

func TestRemoveUntracksButKeepsFile(t *testing.T) {
	m := newMachine(t)
	s := m.init(t)

	m.write(t, ".a", "a\n")
	m.write(t, ".d/b", "b\n")
	m.write(t, ".d/c", "c\n")

	if _, err := s.Add("~/.a", "~/.d"); err != nil {
		t.Fatal(err)
	}

	mustCommit(t, s, "initial")

	removed, err := s.Remove(nil, "~/.d")
	if err != nil || len(removed) != 2 {
		t.Fatalf("remove: %v %v", removed, err)
	}

	expectStatus(t, s, "D  .d/b", "D  .d/c")

	if !m.exists(".d/b") {
		t.Fatal("rm must keep the file on disk")
	}

	if _, err := s.Remove(nil, "~/.d"); err == nil {
		t.Fatal("expected untracked path to be rejected")
	}

	tracked, _ := s.Tracked()
	if len(tracked) != 1 || tracked[0].Name != ".a" {
		t.Fatalf("tracked = %v", tracked)
	}
}

func TestExecutableBitAndSymlink(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("modes and symlinks need a Unix-like system")
	}

	m := newMachine(t)
	s := m.init(t)

	m.write(t, "bin/run", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(m.home, "bin", "run"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink("bin/run", filepath.Join(m.home, "link")); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Add("~/bin", "~/link"); err != nil {
		t.Fatal(err)
	}

	mustCommit(t, s, "initial")
	expectStatus(t, s)

	// A second machine restores the mode and the link.
	remote := createRemote(t, s)
	other := newMachine(t)

	clone, _, err := Clone(other.gitDir, other.home, remote, io.Discard)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}

	info, err := os.Stat(filepath.Join(other.home, "bin", "run"))
	if err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("executable bit lost: %v %v", info, err)
	}

	target, err := os.Readlink(filepath.Join(other.home, "link"))
	if err != nil || target != "bin/run" {
		t.Fatalf("symlink: %q %v", target, err)
	}

	expectStatus(t, clone)
}

// createRemote publishes s to a new bare repository and returns its URL.
func createRemote(t *testing.T, s *Store) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("the file:// transport needs the git binary")
	}

	remotePath := filepath.Join(t.TempDir(), "remote.git")

	_, err := git.PlainInitWithOptions(remotePath, &git.PlainInitOptions{
		Bare:        true,
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName(DefaultBranch)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// file:///D:/x on Windows; "file://D:/x" would make D: the host.
	urlPath := filepath.ToSlash(remotePath)
	if !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}

	url := "file://" + urlPath

	if err := s.AddRemote("origin", url); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Push("origin", io.Discard); err != nil {
		t.Fatalf("push: %v", err)
	}

	return url
}

func TestCloneAndPull(t *testing.T) {
	a := newMachine(t)
	source := a.init(t)

	a.write(t, ".bashrc", "alias ll=ls\n")
	a.write(t, ".config/tool/conf", "v1\n")

	if _, err := source.Add("~/.bashrc", "~/.config/tool"); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, source, "initial")

	remote := createRemote(t, source)

	b := newMachine(t)
	clone, result, err := Clone(b.gitDir, b.home, remote, io.Discard)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}

	if len(result.Written) != 2 || b.read(t, ".bashrc") != "alias ll=ls\n" {
		t.Fatalf("clone result %v", result)
	}

	expectStatus(t, clone)

	// Up to date.
	if result, err := clone.Pull("origin", false, io.Discard); err != nil || result.Updated {
		t.Fatalf("pull without changes: %v %v", result, err)
	}

	// Machine A changes one file, adds one and removes one.
	a.write(t, ".config/tool/conf", "v2\n")
	a.write(t, ".config/tool/new", "n\n")
	if _, err := source.Add("~/.config/tool"); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Remove(nil, "~/.bashrc"); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, source, "update")

	if _, err := source.Push("origin", io.Discard); err != nil {
		t.Fatal(err)
	}

	// Machine B has an unrelated local edit that the update must not touch.
	b.write(t, ".config/tool/other", "mine\n")

	pulled, err := clone.Pull("origin", false, io.Discard)
	if err != nil || !pulled.Updated || pulled.Merged {
		t.Fatalf("pull: %v %v", pulled, err)
	}

	if b.read(t, ".config/tool/conf") != "v2\n" ||
		b.read(t, ".config/tool/new") != "n\n" {
		t.Fatal("pulled files were not written")
	}

	if b.exists(".bashrc") {
		t.Fatal("file removed upstream should be deleted")
	}

	if !b.exists(".config/tool/other") {
		t.Fatal("untracked file must survive")
	}

	expectStatus(t, clone)
}

func TestPullConflictAndForce(t *testing.T) {
	a := newMachine(t)
	source := a.init(t)

	a.write(t, ".rc", "one\n")
	a.write(t, ".keep", "keep\n")
	if _, err := source.Add("~/.rc", "~/.keep"); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, source, "initial")

	remote := createRemote(t, source)

	b := newMachine(t)
	clone, _, err := Clone(b.gitDir, b.home, remote, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	a.write(t, ".rc", "two\n")
	if _, err := source.Add("~/.rc"); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, source, "two")
	if _, err := source.Push("origin", io.Discard); err != nil {
		t.Fatal(err)
	}

	b.write(t, ".rc", "local edit\n")
	b.write(t, ".keep", "local edit\n") // not touched upstream

	_, err = clone.Pull("origin", false, io.Discard)

	var conflict *ConflictError
	if !errors.As(err, &conflict) || strings.Join(conflict.Paths, ",") != ".rc" {
		t.Fatalf("want conflict on .rc, got %v", err)
	}

	if b.read(t, ".rc") != "local edit\n" {
		t.Fatal("conflicting pull must not write anything")
	}

	if _, err := clone.Pull("origin", true, io.Discard); err != nil {
		t.Fatalf("forced pull: %v", err)
	}

	if b.read(t, ".rc") != "two\n" {
		t.Fatal("forced pull should overwrite")
	}

	if b.read(t, ".keep") != "local edit\n" {
		t.Fatal("files untouched upstream keep local edits")
	}

	expectStatus(t, clone, " M .keep")
}

func TestPullRefusesStagedChanges(t *testing.T) {
	a := newMachine(t)
	source := a.init(t)

	a.write(t, ".rc", "one\n")
	if _, err := source.Add("~/.rc"); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, source, "initial")

	remote := createRemote(t, source)

	b := newMachine(t)
	clone, _, err := Clone(b.gitDir, b.home, remote, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	b.write(t, ".extra", "x\n")
	if _, err := clone.Add("~/.extra"); err != nil {
		t.Fatal(err)
	}

	if _, err := clone.Pull("origin", false, io.Discard); !errors.Is(err, ErrPullStagedChanges) {
		t.Fatalf("want ErrPullStagedChanges, got %v", err)
	}
}

func TestCloneConflictKeepsRepositoryUntilForced(t *testing.T) {
	a := newMachine(t)
	source := a.init(t)

	a.write(t, ".rc", "repo\n")
	a.write(t, ".same", "same\n")
	if _, err := source.Add("~/.rc", "~/.same"); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, source, "initial")

	remote := createRemote(t, source)

	b := newMachine(t)
	b.write(t, ".rc", "existing\n")
	b.write(t, ".same", "same\n") // identical content is not a conflict

	clone, _, err := Clone(b.gitDir, b.home, remote, io.Discard)

	var conflict *ConflictError
	if !errors.As(err, &conflict) || strings.Join(conflict.Paths, ",") != ".rc" {
		t.Fatalf("want conflict on .rc, got %v", err)
	}

	if clone == nil || b.read(t, ".rc") != "existing\n" {
		t.Fatal("conflicting clone must keep the repository and the file")
	}

	// The conflicting file must be reviewable, and only that file.
	diffs, err := clone.Diff(false)
	if err != nil || len(diffs) != 1 || diffs[0].Path != ".rc" {
		t.Fatalf("diff after conflicting clone: %v %v", diffs, err)
	}

	if _, err := clone.Checkout(true); err != nil {
		t.Fatalf("forced checkout: %v", err)
	}

	if b.read(t, ".rc") != "repo\n" {
		t.Fatal("forced checkout should overwrite")
	}

	expectStatus(t, clone)
}

func TestCheckoutRestoresDeletedFiles(t *testing.T) {
	m := newMachine(t)
	s := m.init(t)

	m.write(t, ".rc", "one\n")
	if _, err := s.Add("~/.rc"); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, s, "initial")

	if err := os.Remove(filepath.Join(m.home, ".rc")); err != nil {
		t.Fatal(err)
	}

	result, err := s.Checkout(false)
	if err != nil || len(result.Written) != 1 || m.read(t, ".rc") != "one\n" {
		t.Fatalf("checkout: %v %v", result, err)
	}

	m.write(t, ".rc", "changed\n")

	if _, err := s.Checkout(false); !errors.Is(err, ErrConflicts) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestSyncRejectsUnsafePaths(t *testing.T) {
	for _, name := range []string{"../escape", "a/../b", ".git/config", "A/.GIT/x", "/abs", "a//b", "..##os.linux"} {
		if err := validateName(name); err == nil {
			t.Errorf("validateName(%q) should fail", name)
		}
	}

	if err := validateName(".config/git/config"); err != nil {
		t.Errorf("validateName rejected a normal path: %v", err)
	}
}

func TestPrivateFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no permission bits")
	}

	a := newMachine(t)
	source := a.init(t)

	a.write(t, ".ssh/id_ed25519", "key\n")
	a.write(t, ".rc", "rc\n")
	if _, err := source.Add("~/.ssh/id_ed25519", "~/.rc"); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, source, "key")

	b := newMachine(t)
	clone, _, err := Clone(b.gitDir, b.home, createRemote(t, source), io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	perm := func(name string) os.FileMode {
		t.Helper()

		info, err := os.Stat(filepath.Join(b.home, name))
		if err != nil {
			t.Fatal(err)
		}

		return info.Mode().Perm()
	}

	if got := perm(".ssh/id_ed25519"); got != 0o600 {
		t.Errorf("key: %o", got)
	}
	if got := perm(".ssh"); got != 0o700 {
		t.Errorf(".ssh: %o", got)
	}
	if got := perm(".rc"); got != 0o644 {
		t.Errorf(".rc: %o", got)
	}

	// A key that dot 0.4 wrote readable for everyone is made private.
	if err := os.Chmod(filepath.Join(b.home, ".ssh/id_ed25519"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := clone.Checkout(false)
	if err != nil || strings.Join(result.Secured, ",") != ".ssh/id_ed25519" {
		t.Fatalf("checkout: %+v %v", result, err)
	}
	if got := perm(".ssh/id_ed25519"); got != 0o600 {
		t.Errorf("key after checkout: %o", got)
	}
}

func TestLog(t *testing.T) {
	m := newMachine(t)
	s := m.init(t)

	if entries, err := s.Log(0); err != nil || len(entries) != 0 {
		t.Fatalf("empty repository: %v %v", entries, err)
	}

	m.write(t, ".rc", "one\n")
	if _, err := s.Add("~/.rc"); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, s, "first")

	m.write(t, ".rc", "two\n")
	if _, err := s.Add("~/.rc"); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, s, "second")

	entries, err := s.Log(0)
	if err != nil || len(entries) != 2 || strings.TrimSpace(entries[0].Message) != "second" {
		t.Fatalf("history: %v %v", entries, err)
	}

	if len(entries[0].Parents) != 1 || entries[0].Parents[0] != entries[1].Hash {
		t.Errorf("the second commit must have the first as parent: %v", entries[0].Parents)
	}

	if limited, err := s.Log(1); err != nil || len(limited) != 1 {
		t.Fatalf("limit: %v %v", limited, err)
	}
}
