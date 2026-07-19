package gitstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

// Validate verifies that path contains a usable Git repository.
func Validate(path string) error {
	_, err := git.PlainOpen(path)
	if err != nil {
		return fmt.Errorf("open repository at %q: %w", path, err)
	}

	return nil
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

// Stage adds repository-relative paths to the Git index.
//
// Git repository paths always use forward slashes, including on Windows.
func Stage(repositoryPath string, repositoryPaths ...string) error {
	repository, err := git.PlainOpen(repositoryPath)
	if err != nil {
		return fmt.Errorf("open repository at %q: %w", repositoryPath, err)
	}

	worktree, err := repository.Worktree()
	if err != nil {
		return fmt.Errorf("open repository worktree: %w", err)
	}

	for _, repositoryPath := range repositoryPaths {
		normalized := filepath.ToSlash(repositoryPath)

		if _, err := worktree.Add(normalized); err != nil {
			return fmt.Errorf("stage %q: %w", normalized, err)
		}
	}

	return nil
}
