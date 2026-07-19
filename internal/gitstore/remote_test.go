package gitstore

import (
	"path/filepath"
	"testing"
)

func TestAddAndListRemote(t *testing.T) {
	repositoryPath := filepath.Join(
		t.TempDir(),
		"repository",
	)

	if err := Init(repositoryPath); err != nil {
		t.Fatalf("initialize repository: %v", err)
	}

	remoteURL := "git@git.sr.ht:~stephan/dotfiles"

	if err := AddRemote(
		repositoryPath,
		"origin",
		remoteURL,
	); err != nil {
		t.Fatalf("add remote: %v", err)
	}

	remotes, err := ListRemotes(repositoryPath)
	if err != nil {
		t.Fatalf("list remotes: %v", err)
	}

	if len(remotes) != 1 {
		t.Fatalf(
			"expected one remote, got %d",
			len(remotes),
		)
	}

	if remotes[0].Name != "origin" {
		t.Fatalf(
			"expected remote name %q, got %q",
			"origin",
			remotes[0].Name,
		)
	}

	if len(remotes[0].URLs) != 1 {
		t.Fatalf(
			"expected one remote URL, got %d",
			len(remotes[0].URLs),
		)
	}

	if remotes[0].URLs[0] != remoteURL {
		t.Fatalf(
			"expected URL %q, got %q",
			remoteURL,
			remotes[0].URLs[0],
		)
	}
}

func TestAddRemoteRejectsDuplicateName(t *testing.T) {
	repositoryPath := filepath.Join(
		t.TempDir(),
		"repository",
	)

	if err := Init(repositoryPath); err != nil {
		t.Fatalf("initialize repository: %v", err)
	}

	if err := AddRemote(
		repositoryPath,
		"origin",
		"https://example.org/first.git",
	); err != nil {
		t.Fatalf("add first remote: %v", err)
	}

	err := AddRemote(
		repositoryPath,
		"origin",
		"https://example.org/second.git",
	)
	if err == nil {
		t.Fatal("expected duplicate remote to be rejected")
	}
}

func TestListRemotesReturnsEmptyList(t *testing.T) {
	repositoryPath := filepath.Join(
		t.TempDir(),
		"repository",
	)

	if err := Init(repositoryPath); err != nil {
		t.Fatalf("initialize repository: %v", err)
	}

	remotes, err := ListRemotes(repositoryPath)
	if err != nil {
		t.Fatalf("list remotes: %v", err)
	}

	if len(remotes) != 0 {
		t.Fatalf(
			"expected no remotes, got %d",
			len(remotes),
		)
	}
}
