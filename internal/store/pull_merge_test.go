package store

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

// divergedSetup is a pair of machines that share a first commit with .rc and
// .keep. Both hold a clone, so each test creates the differing commits.
type divergedSetup struct {
	remote string
	a      machine
	source *Store
	b      machine
	clone  *Store
}

func newDivergedSetup(t *testing.T) divergedSetup {
	t.Helper()

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

	for key, value := range map[string]string{
		"user.name":  "Test",
		"user.email": "test@example.com",
	} {
		if err := clone.SetConfig(key, value); err != nil {
			t.Fatal(err)
		}
	}

	return divergedSetup{remote: remote, a: a, source: source, b: b, clone: clone}
}

// commitOn changes a file on the given machine and commits it.
func commitOn(t *testing.T, m machine, s *Store, name string, content string) {
	t.Helper()

	m.write(t, name, content)
	if _, err := s.Add("~/" + name); err != nil {
		t.Fatal(err)
	}
	// The machine in the message keeps equal changes on two machines from
	// producing the very same commit.
	mustCommit(t, s, "change "+name+" on "+m.home)
}

func (d divergedSetup) pushRemoteChange(t *testing.T, name string, content string) {
	t.Helper()

	commitOn(t, d.a, d.source, name, content)
	if _, err := d.source.Push("origin", io.Discard); err != nil {
		t.Fatal(err)
	}
}

func headParents(t *testing.T, s *Store) []plumbing.Hash {
	t.Helper()

	head, err := s.repo.Head()
	if err != nil {
		t.Fatal(err)
	}

	commit, err := s.repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatal(err)
	}

	return commit.ParentHashes
}

func TestPullMergesChangesToDifferentFiles(t *testing.T) {
	d := newDivergedSetup(t)

	commitOn(t, d.b, d.clone, ".extra", "local\n")
	d.pushRemoteChange(t, ".rc", "two\n")

	result, err := d.clone.Pull("origin", false, io.Discard)
	if err != nil || !result.Updated || !result.Merged {
		t.Fatalf("pull: %+v %v", result, err)
	}

	if d.b.read(t, ".rc") != "two\n" || d.b.read(t, ".extra") != "local\n" {
		t.Fatal("merge lost a change")
	}

	if len(headParents(t, d.clone)) != 2 {
		t.Fatal("merge must create a commit with two parents")
	}

	expectStatus(t, d.clone)

	// The merge commit can be pushed, which a rejected pull could not.
	if _, err := d.clone.Push("origin", io.Discard); err != nil {
		t.Fatalf("push after merge: %v", err)
	}
}

func TestPullMergesIdenticalChanges(t *testing.T) {
	d := newDivergedSetup(t)

	commitOn(t, d.b, d.clone, ".rc", "same\n")
	d.pushRemoteChange(t, ".rc", "same\n")

	result, err := d.clone.Pull("origin", false, io.Discard)
	if err != nil || !result.Merged {
		t.Fatalf("pull: %+v %v", result, err)
	}

	if d.b.read(t, ".rc") != "same\n" {
		t.Fatal("identical change was lost")
	}

	expectStatus(t, d.clone)
}

func TestPullMergesRemoteDeletion(t *testing.T) {
	d := newDivergedSetup(t)

	commitOn(t, d.b, d.clone, ".rc", "local\n")

	if _, err := d.source.Remove(nil, "~/.keep"); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, d.source, "remove .keep")
	if _, err := d.source.Push("origin", io.Discard); err != nil {
		t.Fatal(err)
	}

	if _, err := d.clone.Pull("origin", false, io.Discard); err != nil {
		t.Fatal(err)
	}

	if d.b.exists(".keep") || d.b.read(t, ".rc") != "local\n" {
		t.Fatal("deletion and local change must both apply")
	}

	expectStatus(t, d.clone)
}

func TestPullReportsConflictingCommitsAndChangesNothing(t *testing.T) {
	d := newDivergedSetup(t)

	commitOn(t, d.b, d.clone, ".rc", "local\n")
	d.pushRemoteChange(t, ".rc", "remote\n")

	before, err := d.clone.repo.Head()
	if err != nil {
		t.Fatal(err)
	}

	_, err = d.clone.Pull("origin", false, io.Discard)

	var conflict *MergeConflictError
	if !errors.As(err, &conflict) || strings.Join(conflict.Paths, ",") != ".rc" {
		t.Fatalf("want merge conflict on .rc, got %v", err)
	}

	after, err := d.clone.repo.Head()
	if err != nil || after.Hash() != before.Hash() {
		t.Fatalf("branch moved despite the conflict: %v", err)
	}

	if d.b.read(t, ".rc") != "local\n" {
		t.Fatal("conflicting merge must not write anything")
	}
}

func TestPullMergeKeepsLocalEditsOfOtherFilesAndRefusesOverwrites(t *testing.T) {
	d := newDivergedSetup(t)

	commitOn(t, d.b, d.clone, ".extra", "local\n")
	d.pushRemoteChange(t, ".rc", "two\n")

	d.b.write(t, ".rc", "uncommitted\n")

	_, err := d.clone.Pull("origin", false, io.Discard)

	var conflict *ConflictError
	if !errors.As(err, &conflict) || strings.Join(conflict.Paths, ",") != ".rc" {
		t.Fatalf("want conflict on .rc, got %v", err)
	}

	if len(headParents(t, d.clone)) != 1 {
		t.Fatal("no merge commit may exist after a refused merge")
	}

	if d.b.read(t, ".rc") != "uncommitted\n" {
		t.Fatal("local edit was overwritten")
	}
}

func TestPullWithoutIdentityLeavesMergeUndone(t *testing.T) {
	d := newDivergedSetup(t)

	commitOn(t, d.b, d.clone, ".extra", "local\n")
	d.pushRemoteChange(t, ".rc", "two\n")

	configuration, err := d.clone.repo.Config()
	if err != nil {
		t.Fatal(err)
	}

	configuration.User.Name = ""
	configuration.Raw.Section("user").RemoveOption("name")
	if err := d.clone.repo.SetConfig(configuration); err != nil {
		t.Fatal(err)
	}

	if _, err := d.clone.Pull("origin", false, io.Discard); err == nil {
		t.Fatal("merge without identity must fail")
	}

	if d.b.read(t, ".rc") != "one\n" {
		t.Fatal("files changed although the merge commit could not be made")
	}
}

func TestPullWhenLocalIsAheadChangesNothing(t *testing.T) {
	d := newDivergedSetup(t)

	commitOn(t, d.b, d.clone, ".extra", "local\n")

	result, err := d.clone.Pull("origin", false, io.Discard)
	if err != nil || result.Updated {
		t.Fatalf("pull with only local commits: %+v %v", result, err)
	}
}

func TestMergeFilesTable(t *testing.T) {
	entry := func(n byte) treeEntry {
		return treeEntry{hash: plumbing.Hash{n}, mode: 0o100644}
	}

	tests := []struct {
		name                string
		base, local, remote map[string]treeEntry
		want                map[string]treeEntry
		conflicts           []string
	}{
		{
			name:   "only remote changes",
			base:   map[string]treeEntry{"a": entry(1)},
			local:  map[string]treeEntry{"a": entry(1)},
			remote: map[string]treeEntry{"a": entry(2)},
			want:   map[string]treeEntry{"a": entry(2)},
		},
		{
			name:   "added on both sides identically",
			base:   map[string]treeEntry{},
			local:  map[string]treeEntry{"a": entry(1)},
			remote: map[string]treeEntry{"a": entry(1)},
			want:   map[string]treeEntry{"a": entry(1)},
		},
		{
			name:      "added on both sides differently",
			base:      map[string]treeEntry{},
			local:     map[string]treeEntry{"a": entry(1)},
			remote:    map[string]treeEntry{"a": entry(2)},
			want:      map[string]treeEntry{},
			conflicts: []string{"a"},
		},
		{
			name:      "deleted remotely but changed locally",
			base:      map[string]treeEntry{"a": entry(1)},
			local:     map[string]treeEntry{"a": entry(2)},
			remote:    map[string]treeEntry{},
			want:      map[string]treeEntry{},
			conflicts: []string{"a"},
		},
		{
			name:   "deleted on both sides",
			base:   map[string]treeEntry{"a": entry(1)},
			local:  map[string]treeEntry{},
			remote: map[string]treeEntry{},
			want:   map[string]treeEntry{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, conflicts := mergeFiles(test.base, test.local, test.remote)

			if len(got) != len(test.want) {
				t.Fatalf("merged %v, want %v", got, test.want)
			}

			for name, entry := range test.want {
				if got[name] != entry {
					t.Fatalf("merged %v, want %v", got, test.want)
				}
			}

			if strings.Join(conflicts, ",") != strings.Join(test.conflicts, ",") {
				t.Fatalf("conflicts %v, want %v", conflicts, test.conflicts)
			}
		})
	}
}
