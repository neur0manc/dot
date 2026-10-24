package store

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// otherOS is an operating system that the tests do not run on.
func otherOS() string {
	if runtime.GOOS == "linux" {
		return "darwin"
	}

	return "linux"
}

func sortedLines(lines ...string) []string {
	slices.SortFunc(lines, func(a, b string) int { return strings.Compare(a[3:], b[3:]) })
	return lines
}

func TestSplitName(t *testing.T) {
	for name, want := range map[string]string{
		".zshrc":                         ".zshrc|",
		".zshrc##os.darwin":              ".zshrc|os.darwin",
		".config/x/y##os.linux,class.ws": ".config/x/y|os.linux,class.ws",
		"dir##os.linux/file":             "dir##os.linux/file|",
		"##os.linux":                     "##os.linux|",
		".zshrc##":                       ".zshrc##|",
	} {
		filePath, conditions := SplitName(name)
		if got := filePath + "|" + strings.Join(conditions, ","); got != want {
			t.Errorf("SplitName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestShowPicksTheBestVariant(t *testing.T) {
	files := map[string]treeEntry{}
	for _, name := range []string{
		"a", "a##os.linux", "a##os.unix", "a##os.linux,class.work", "a##hostname.box",
		"b##os.windows",
		"c", "c##os.macos",
		"d##arch.x86_64,os.unix",
	} {
		files[name] = treeEntry{}
	}

	for _, test := range []struct {
		facts Facts
		want  string
	}{
		{Facts{OS: "linux", Arch: "amd64"}, "a##os.linux c d##arch.x86_64,os.unix"},
		{Facts{OS: "linux", Class: "work"}, "a##os.linux,class.work c"},
		{Facts{OS: "LINUX", Hostname: "Box"}, "a##hostname.box c"},
		{Facts{OS: "darwin"}, "a##os.unix c##os.macos"},
		{Facts{OS: "windows"}, "a b##os.windows c"},
	} {
		shown := test.facts.show(files)

		var got []string
		for _, filePath := range []string{"a", "b", "c", "d"} {
			if entry, ok := shown[filePath]; ok {
				got = append(got, entry.name)
			}
		}

		if strings.Join(got, " ") != test.want {
			t.Errorf("%+v: shown %q, want %q", test.facts, got, test.want)
		}
	}
}

func TestCondition(t *testing.T) {
	facts := Facts{OS: "darwin", Hostname: "mac"}

	for _, test := range []struct{ key, value, want string }{
		{ConditionOS, "", "os.darwin"},
		{ConditionOS, "Linux", "os.linux"},
		{ConditionHostname, "", "hostname.mac"},
		{ConditionClass, "server", "class.server"},
	} {
		if got, err := facts.Condition(test.key, test.value); err != nil || got != test.want {
			t.Errorf("Condition(%q, %q) = %q, %v", test.key, test.value, got, err)
		}
	}

	for _, test := range []struct{ key, value string }{
		{ConditionClass, ""}, // no class configured
		{ConditionOS, "a,b"},
		{ConditionHostname, "../x"},
	} {
		if _, err := facts.Condition(test.key, test.value); err == nil {
			t.Errorf("Condition(%q, %q) should fail", test.key, test.value)
		}
	}
}

func TestVariantsAcrossMachines(t *testing.T) {
	here, there := "os."+runtime.GOOS, "os."+otherOS()

	a := newMachine(t)
	source := a.init(t)
	source.facts.OS = otherOS() // machine A runs the other system

	a.write(t, ".zshrc", "there\n")
	if _, err := source.AddVariant([]string{there}, "~/.zshrc"); err != nil {
		t.Fatal(err)
	}

	a.write(t, ".zshrc", "here\n")
	added, err := source.AddVariant([]string{here}, "~/.zshrc")
	if err != nil || added[0].Used {
		t.Fatalf("add: %+v %v", added, err)
	}
	a.write(t, ".zshrc", "there\n")

	expectStatus(t, source, sortedLines("A  .zshrc##"+here, "A  .zshrc##"+there)...)
	mustCommit(t, source, "variants")

	remote := createRemote(t, source)

	b := newMachine(t)
	clone, result, err := Clone(b.gitDir, b.home, remote, io.Discard)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}

	if strings.Join(result.Written, ",") != ".zshrc" || b.read(t, ".zshrc") != "here\n" {
		t.Fatalf("clone wrote %v, .zshrc = %q", result.Written, b.read(t, ".zshrc"))
	}
	if b.exists(".zshrc##" + here) {
		t.Fatal("a variant file was written")
	}
	expectStatus(t, clone)

	// A change to the other system's variant leaves this machine alone.
	a.write(t, ".zshrc", "there 2\n")
	if _, err := source.Add("~/.zshrc"); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, source, "there 2")
	if _, err := source.Push("origin", io.Discard); err != nil {
		t.Fatal(err)
	}

	before, _ := os.Stat(filepath.Join(b.home, ".zshrc"))
	time.Sleep(10 * time.Millisecond)

	if _, err := clone.Pull("origin", false, io.Discard); err != nil {
		t.Fatal(err)
	}

	after, _ := os.Stat(filepath.Join(b.home, ".zshrc"))
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal(".zshrc was rewritten")
	}

	// Edits on this machine go to its variant.
	b.write(t, ".zshrc", "here 2\n")
	expectStatus(t, clone, " M .zshrc##"+here)

	added, err = clone.Add("~/.zshrc")
	if err != nil || added[0].Name != ".zshrc##"+here || !added[0].Used {
		t.Fatalf("add: %+v %v", added, err)
	}
}

func TestAddVariantLimitsAPlainFile(t *testing.T) {
	m := newMachine(t)
	s := m.init(t)

	m.write(t, ".rc", "rc\n")
	if _, err := s.Add("~/.rc"); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, s, "plain")

	condition := "hostname." + strings.ToLower(s.facts.Hostname)

	added, err := s.AddVariant([]string{condition}, "~/.rc")
	if err != nil || added[0].Replaces != ".rc" || !added[0].Used {
		t.Fatalf("add: %+v %v", added, err)
	}

	expectStatus(t, s, "D  .rc", "A  .rc##"+condition)

	// A variant for other machines is added next to what is there.
	added, err = s.AddVariant([]string{"os." + otherOS()}, "~/.rc")
	if err != nil || added[0].Replaces != "" || added[0].Used {
		t.Fatalf("add: %+v %v", added, err)
	}

	// rm removes the variant used here unless told otherwise.
	removed, err := s.Remove(nil, "~/.rc")
	if err != nil || strings.Join(removed, ",") != ".rc##"+condition {
		t.Fatalf("rm: %v %v", removed, err)
	}

	if _, err := s.Remove(nil, "~/.rc"); err == nil {
		t.Fatal("rm without conditions should fail when no variant is used here")
	}

	if _, err := s.Remove([]string{"os." + otherOS()}, "~/.rc"); err != nil {
		t.Fatal(err)
	}
}

func TestAddRejectsVariantNamesOnDisk(t *testing.T) {
	m := newMachine(t)
	s := m.init(t)

	m.write(t, ".rc##os.linux", "x\n")
	m.write(t, ".d/f##os.linux", "x\n")

	for _, path := range []string{"~/.rc##os.linux", "~/.d"} {
		if _, err := s.Add(path); err == nil {
			t.Errorf("add %s should fail", path)
		}
	}
}

func TestClassChangeSwitchesUnmodifiedFiles(t *testing.T) {
	m := newMachine(t)
	s := m.init(t)

	m.write(t, ".rc", "default\n")
	m.write(t, ".other", "other\n")
	if _, err := s.Add("~/.rc", "~/.other"); err != nil {
		t.Fatal(err)
	}

	m.write(t, ".rc", "work\n")
	if _, err := s.AddVariant([]string{"class.work"}, "~/.rc"); err != nil {
		t.Fatal(err)
	}
	m.write(t, ".rc", "default\n")
	mustCommit(t, s, "initial")

	if err := s.SetConfig(ClassConfigKey, "work"); err != nil {
		t.Fatal(err)
	}

	m.write(t, ".other", "edited\n")

	result, err := s.Checkout(false)
	if !strings.Contains(err.Error(), ".other") {
		t.Fatalf("want a conflict on .other, got %v %v", result, err)
	}

	m.write(t, ".other", "other\n")

	if _, err := s.Checkout(false); err != nil || m.read(t, ".rc") != "work\n" {
		t.Fatalf("checkout: %v, .rc = %q", err, m.read(t, ".rc"))
	}

	expectStatus(t, s)

	// A local edit blocks switching back.
	m.write(t, ".rc", "mine\n")
	if err := s.SetConfig(ClassConfigKey, "home"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Checkout(false); err == nil {
		t.Fatal("expected a conflict")
	}
}

func TestCheckoutKeepsNewlyAddedFiles(t *testing.T) {
	m := newMachine(t)
	s := m.init(t)

	m.write(t, ".a", "a\n")
	if _, err := s.Add("~/.a"); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, s, "initial")

	m.write(t, ".new", "new\n")
	if _, err := s.Add("~/.new"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Checkout(false); err != nil {
		t.Fatal(err)
	}

	if !m.exists(".new") {
		t.Fatal("checkout deleted a newly added file")
	}
	expectStatus(t, s, "A  .new")
}

func TestUpgradeLinkedVariants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("dotmgr 0.3 used copies on Windows")
	}

	m := newMachine(t)
	s := m.init(t)

	here := ".rc##os." + runtime.GOOS
	unused := ".rc##os." + otherOS()
	edited := ".rc##hostname.elsewhere"

	// The layout dotmgr 0.3 left behind.
	m.write(t, here, "here\n")
	m.write(t, unused, "unused\n")
	m.write(t, edited, "edited\n")

	idx, err := s.loadIndex()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{here, unused, edited} {
		file, err := s.readWorktree(name, 0)
		if err != nil {
			t.Fatal(err)
		}
		hash, err := s.writeBlob(file.data)
		if err != nil {
			t.Fatal(err)
		}
		setEntry(idx, name, hash, file.mode, file.size, file.modTime)
	}
	if err := s.saveIndex(idx); err != nil {
		t.Fatal(err)
	}

	m.write(t, edited, "edited later\n")
	m.write(t, here, "here, edited through the link\n")

	if err := os.Symlink(here, filepath.Join(m.home, ".rc")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.gitDir, legacyStateFile), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	notes, err := s.UpgradeLinkedVariants()
	if err != nil || len(notes) != 3 {
		t.Fatalf("upgrade: %q %v", notes, err)
	}

	info, err := os.Lstat(filepath.Join(m.home, ".rc"))
	if err != nil || !info.Mode().IsRegular() || m.read(t, ".rc") != "here, edited through the link\n" {
		t.Fatalf(".rc: %v %v", info, err)
	}

	if m.exists(here) || m.exists(unused) || !m.exists(edited) {
		t.Fatal("variant files were not cleaned up as expected")
	}

	expectStatus(t, s, sortedLines("AM "+here, "A  "+edited, "A  "+unused)...)

	if notes, err := s.UpgradeLinkedVariants(); err != nil || notes != nil {
		t.Fatalf("second upgrade: %q %v", notes, err)
	}
}
