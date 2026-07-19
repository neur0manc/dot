package gitstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCommitCreatesCommitFromStagedChanges(t *testing.T) {
	repositoryPath := filepath.Join(t.TempDir(), "repository")

	if err := Init(repositoryPath); err != nil {
		t.Fatalf("initialize repository: %v", err)
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

	hash, err := Commit(repositoryPath, "Add example file")
	if err != nil {
		t.Fatalf("commit staged file: %v", err)
	}

	if hash.IsZero() {
		t.Fatal("expected a non-zero commit hash")
	}

	status, err := Status(repositoryPath)
	if err != nil {
		t.Fatalf("read repository status: %v", err)
	}

	if !status.IsClean() {
		t.Fatalf("expected clean repository, got:\n%s", status)
	}
}

func TestCommitRequiresIdentity(t *testing.T) {
	repositoryPath := filepath.Join(t.TempDir(), "repository")

	if err := Init(repositoryPath); err != nil {
		t.Fatalf("initialize repository: %v", err)
	}

	_, err := Commit(repositoryPath, "Initial commit")
	if err == nil {
		t.Fatal("expected missing identity to be rejected")
	}
}

func TestCommitRejectsRepositoryWithoutStagedChanges(t *testing.T) {
	repositoryPath := filepath.Join(t.TempDir(), "repository")

	if err := Init(repositoryPath); err != nil {
		t.Fatalf("initialize repository: %v", err)
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

	// Init stages .gitignore, so create the initial commit first.
	if _, err := Commit(repositoryPath, "Initial commit"); err != nil {
		t.Fatalf("create initial commit: %v", err)
	}

	_, err := Commit(repositoryPath, "Empty commit")
	if !errors.Is(err, ErrNoStagedChanges) {
		t.Fatalf(
			"expected ErrNoStagedChanges, got %v",
			err,
		)
	}
}
