package gitstore

import (
	"errors"
	"fmt"
	"os"

	git "github.com/go-git/go-git/v5"
)

// Init creates a non-bare Git repository at path.
func Init(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create repository directory: %w", err)
	}

	_, err := git.PlainInit(path, false)
	if err == nil {
		return nil
	}

	if errors.Is(err, git.ErrRepositoryAlreadyExists) {
		return fmt.Errorf("repository already exists at %q", path)
	}

	return fmt.Errorf("initialize Git repository: %w", err)
}

// Status returns the current Git worktree status.
func Status(path string) (git.Status, error) {
	repository, err := git.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("open repository at %q: %w", path, err)
	}

	worktree, err := repository.Worktree()
	if err != nil {
		return nil, fmt.Errorf("open repository worktree: %w", err)
	}

	status, err := worktree.Status()
	if err != nil {
		return nil, fmt.Errorf("read repository status: %w", err)
	}

	return status, nil
}
