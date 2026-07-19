package gitstore

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
)

func TestPushCurrentBranchToLocalBareRepository(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repository")
	remotePath := filepath.Join(root, "remote.git")

	if err := Init(repositoryPath); err != nil {
		t.Fatalf("initialize repository: %v", err)
	}

	if _, err := git.PlainInit(remotePath, true); err != nil {
		t.Fatalf("initialize bare remote: %v", err)
	}

	if err := SetUserConfig(
		repositoryPath,
		"user.name",
		"Test User",
	); err != nil {
		t.Fatalf("set user name: %v", err)
	}

	if err := SetUserConfig(
		repositoryPath,
		"user.email",
		"test@example.com",
	); err != nil {
		t.Fatalf("set user email: %v", err)
	}

	filename := filepath.Join(repositoryPath, "example.txt")

	if err := os.WriteFile(
		filename,
		[]byte("example\n"),
		0o644,
	); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	if err := Stage(repositoryPath, "example.txt"); err != nil {
		t.Fatalf("stage test file: %v", err)
	}

	if _, err := Commit(
		repositoryPath,
		"Add example file",
	); err != nil {
		t.Fatalf("commit test file: %v", err)
	}

	if err := AddRemote(
		repositoryPath,
		"origin",
		remotePath,
	); err != nil {
		t.Fatalf("add local remote: %v", err)
	}

	pushed, err := Push(
		repositoryPath,
		"origin",
		io.Discard,
	)
	if err != nil {
		t.Fatalf("push repository: %v", err)
	}

	if !pushed {
		t.Fatal("expected the first push to transfer changes")
	}

	localRepository, err := git.PlainOpen(repositoryPath)
	if err != nil {
		t.Fatalf("open local repository: %v", err)
	}

	localHead, err := localRepository.Head()
	if err != nil {
		t.Fatalf("resolve local HEAD: %v", err)
	}

	remoteRepository, err := git.PlainOpen(remotePath)
	if err != nil {
		t.Fatalf("open remote repository: %v", err)
	}

	remoteReference, err := remoteRepository.Reference(
		localHead.Name(),
		true,
	)
	if err != nil {
		t.Fatalf("resolve remote branch: %v", err)
	}

	if remoteReference.Hash() != localHead.Hash() {
		t.Fatalf(
			"expected remote hash %s, got %s",
			localHead.Hash(),
			remoteReference.Hash(),
		)
	}

	pushed, err = Push(
		repositoryPath,
		"origin",
		io.Discard,
	)
	if err != nil {
		t.Fatalf("repeat push: %v", err)
	}

	if pushed {
		t.Fatal("expected repeated push to be up to date")
	}
}
