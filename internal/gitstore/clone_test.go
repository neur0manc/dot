package gitstore

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
)

func TestCloneLocalRepository(t *testing.T) {
	root := t.TempDir()

	sourcePath := filepath.Join(root, "source")
	remotePath := filepath.Join(root, "remote.git")
	clonePath := filepath.Join(root, "clone")

	if err := Init(sourcePath); err != nil {
		t.Fatalf("initialize source repository: %v", err)
	}

	if err := SetUserConfig(
		sourcePath,
		"user.name",
		"Test User",
	); err != nil {
		t.Fatalf("set user name: %v", err)
	}

	if err := SetUserConfig(
		sourcePath,
		"user.email",
		"test@example.com",
	); err != nil {
		t.Fatalf("set user email: %v", err)
	}

	filename := filepath.Join(sourcePath, "example.txt")

	if err := os.WriteFile(
		filename,
		[]byte("example\n"),
		0o644,
	); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	if err := Stage(sourcePath, "example.txt"); err != nil {
		t.Fatalf("stage source file: %v", err)
	}

	sourceHash, err := Commit(
		sourcePath,
		"Add example file",
	)
	if err != nil {
		t.Fatalf("commit source file: %v", err)
	}

	if _, err := git.PlainInit(remotePath, true); err != nil {
		t.Fatalf("initialize bare remote: %v", err)
	}

	if err := AddRemote(
		sourcePath,
		"origin",
		remotePath,
	); err != nil {
		t.Fatalf("add source remote: %v", err)
	}

	pushed, err := Push(
		sourcePath,
		"origin",
		io.Discard,
	)
	if err != nil {
		t.Fatalf("push source repository: %v", err)
	}

	if !pushed {
		t.Fatal("expected source repository to be pushed")
	}

	if err := Clone(
		clonePath,
		remotePath,
		io.Discard,
	); err != nil {
		t.Fatalf("clone repository: %v", err)
	}

	clonedRepository, err := git.PlainOpen(clonePath)
	if err != nil {
		t.Fatalf("open cloned repository: %v", err)
	}

	clonedHead, err := clonedRepository.Head()
	if err != nil {
		t.Fatalf("resolve cloned HEAD: %v", err)
	}

	if clonedHead.Hash() != sourceHash {
		t.Fatalf(
			"expected cloned hash %s, got %s",
			sourceHash,
			clonedHead.Hash(),
		)
	}

	clonedData, err := os.ReadFile(
		filepath.Join(clonePath, "example.txt"),
	)
	if err != nil {
		t.Fatalf("read cloned file: %v", err)
	}

	if string(clonedData) != "example\n" {
		t.Fatalf(
			"expected cloned content %q, got %q",
			"example\n",
			string(clonedData),
		)
	}

	remotes, err := ListRemotes(clonePath)
	if err != nil {
		t.Fatalf("list cloned remotes: %v", err)
	}

	if len(remotes) != 1 {
		t.Fatalf(
			"expected one cloned remote, got %d",
			len(remotes),
		)
	}

	if remotes[0].Name != "origin" {
		t.Fatalf(
			"expected remote %q, got %q",
			"origin",
			remotes[0].Name,
		)
	}
}

func TestCloneAllowsExistingEmptyDirectory(t *testing.T) {
	root := t.TempDir()

	sourcePath := filepath.Join(root, "source")
	remotePath := filepath.Join(root, "remote.git")
	clonePath := filepath.Join(root, "empty-clone")

	if err := Init(sourcePath); err != nil {
		t.Fatalf("initialize source repository: %v", err)
	}

	if err := SetUserConfig(
		sourcePath,
		"user.name",
		"Test User",
	); err != nil {
		t.Fatalf("set user name: %v", err)
	}

	if err := SetUserConfig(
		sourcePath,
		"user.email",
		"test@example.com",
	); err != nil {
		t.Fatalf("set user email: %v", err)
	}

	if _, err := Commit(
		sourcePath,
		"Initial commit",
	); err != nil {
		t.Fatalf("create source commit: %v", err)
	}

	if _, err := git.PlainInit(remotePath, true); err != nil {
		t.Fatalf("initialize bare remote: %v", err)
	}

	if err := AddRemote(
		sourcePath,
		"origin",
		remotePath,
	); err != nil {
		t.Fatalf("add source remote: %v", err)
	}

	if _, err := Push(
		sourcePath,
		"origin",
		io.Discard,
	); err != nil {
		t.Fatalf("push source repository: %v", err)
	}

	if err := os.MkdirAll(clonePath, 0o755); err != nil {
		t.Fatalf("create empty destination: %v", err)
	}

	if err := Clone(
		clonePath,
		remotePath,
		io.Discard,
	); err != nil {
		t.Fatalf(
			"clone into existing empty directory: %v",
			err,
		)
	}

	if _, err := git.PlainOpen(clonePath); err != nil {
		t.Fatalf("open cloned repository: %v", err)
	}
}

func TestCloneRejectsNonEmptyDestination(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "repository")

	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatalf("create destination: %v", err)
	}

	markerPath := filepath.Join(destination, "keep.txt")

	if err := os.WriteFile(
		markerPath,
		[]byte("do not remove\n"),
		0o644,
	); err != nil {
		t.Fatalf("write marker file: %v", err)
	}

	err := Clone(
		destination,
		filepath.Join(root, "unused.git"),
		io.Discard,
	)
	if !errors.Is(err, ErrRepositoryPathNotEmpty) {
		t.Fatalf(
			"expected ErrRepositoryPathNotEmpty, got %v",
			err,
		)
	}

	data, readErr := os.ReadFile(markerPath)
	if readErr != nil {
		t.Fatalf("read preserved marker: %v", readErr)
	}

	if string(data) != "do not remove\n" {
		t.Fatalf(
			"destination content changed to %q",
			string(data),
		)
	}
}
