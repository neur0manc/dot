package store

import (
	"io"
	"slices"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

// publishedStore returns a committed store whose branch was pushed to origin.
func publishedStore(t *testing.T) *Store {
	t.Helper()

	m := newMachine(t)
	s := m.init(t)

	m.write(t, "a.txt", "a\n")

	if _, err := s.Add("~/a.txt"); err != nil {
		t.Fatal(err)
	}

	mustCommit(t, s, "first")
	createRemote(t, s)

	return s
}

func remoteNames(t *testing.T, s *Store) []string {
	t.Helper()

	remotes, err := s.Remotes()
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, remote := range remotes {
		names = append(names, remote.Name)
	}

	return names
}

func resolved(t *testing.T, s *Store) string {
	t.Helper()

	name, err := s.ResolveRemote("")
	if err != nil {
		t.Fatal(err)
	}

	return name
}

func TestFirstPushSetsUpstreamButLaterPushesKeepIt(t *testing.T) {
	s := publishedStore(t)

	remotes, err := s.Remotes()
	if err != nil {
		t.Fatal(err)
	}

	if err := s.AddRemote("imwald.org", remotes[0].URLs[0]); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Push("imwald.org", io.Discard); err != nil {
		t.Fatal(err)
	}

	if got := resolved(t, s); got != "origin" {
		t.Fatalf("default remote = %q, want origin", got)
	}

	if name, _ := s.ResolveRemote("imwald.org"); name != "imwald.org" {
		t.Fatalf("explicit remote = %q", name)
	}

	if err := s.SetUpstream("imwald.org"); err != nil {
		t.Fatal(err)
	}

	if got := resolved(t, s); got != "imwald.org" {
		t.Fatalf("default remote = %q, want imwald.org", got)
	}

	if err := s.SetUpstream("missing"); err == nil {
		t.Fatal("SetUpstream accepted an unknown remote")
	}
}

func TestResolveRemoteFallsBackToOrigin(t *testing.T) {
	m := newMachine(t)
	s := m.init(t)

	if got := resolved(t, s); got != "origin" {
		t.Fatalf("default remote = %q, want origin", got)
	}
}

func TestSetRemoteURL(t *testing.T) {
	s := publishedStore(t)

	if err := s.SetRemoteURL("origin", " https://example.org/dots.git "); err != nil {
		t.Fatal(err)
	}

	remotes, _ := s.Remotes()
	if want := []string{"https://example.org/dots.git"}; !slices.Equal(remotes[0].URLs, want) {
		t.Fatalf("URLs = %v, want %v", remotes[0].URLs, want)
	}

	if err := s.SetRemoteURL("origin", " "); err == nil {
		t.Fatal("empty URL accepted")
	}

	if err := s.SetRemoteURL("missing", "x"); err == nil {
		t.Fatal("unknown remote accepted")
	}
}

func TestRenameRemoteMovesUpstreamAndTrackingBranches(t *testing.T) {
	s := publishedStore(t)

	head, err := s.repo.Head()
	if err != nil {
		t.Fatal(err)
	}

	tracking := plumbing.NewRemoteReferenceName("origin", head.Name().Short())
	if err := s.repo.Storer.SetReference(plumbing.NewHashReference(tracking, head.Hash())); err != nil {
		t.Fatal(err)
	}

	if err := s.RenameRemote("origin", "backup"); err != nil {
		t.Fatal(err)
	}

	if got := remoteNames(t, s); !slices.Equal(got, []string{"backup"}) {
		t.Fatalf("remotes = %v", got)
	}

	if got := resolved(t, s); got != "backup" {
		t.Fatalf("default remote = %q, want backup", got)
	}

	moved := plumbing.NewRemoteReferenceName("backup", head.Name().Short())
	if _, err := s.repo.Reference(moved, true); err != nil {
		t.Fatalf("tracking branch not moved: %v", err)
	}

	if _, err := s.repo.Reference(tracking, true); err == nil {
		t.Fatal("old tracking branch still exists")
	}

	if err := s.RenameRemote("backup", ""); err == nil {
		t.Fatal("empty name accepted")
	}

	if err := s.RenameRemote("missing", "x"); err == nil {
		t.Fatal("unknown remote accepted")
	}
}

func TestRenameRemoteRefusesExistingName(t *testing.T) {
	s := publishedStore(t)

	if err := s.AddRemote("backup", "https://example.org/a.git"); err != nil {
		t.Fatal(err)
	}

	if err := s.RenameRemote("backup", "origin"); err == nil {
		t.Fatal("renamed onto an existing remote")
	}

	if got := remoteNames(t, s); !slices.Equal(got, []string{"backup", "origin"}) {
		t.Fatalf("remotes = %v", got)
	}
}

func TestRemoveRemoteDropsUpstream(t *testing.T) {
	s := publishedStore(t)

	if err := s.AddRemote("backup", "https://example.org/a.git"); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveRemote("origin"); err != nil {
		t.Fatal(err)
	}

	if got := remoteNames(t, s); !slices.Equal(got, []string{"backup"}) {
		t.Fatalf("remotes = %v", got)
	}

	// The upstream is gone, so the fallback applies.
	if got := resolved(t, s); got != "origin" {
		t.Fatalf("default remote = %q, want origin", got)
	}

	if err := s.RemoveRemote("origin"); err == nil {
		t.Fatal("removed a remote twice")
	}
}
