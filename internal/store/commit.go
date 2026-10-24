package store

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ErrNoStagedChanges is returned by Commit when there is nothing to commit.
var ErrNoStagedChanges = errors.New("no staged changes")

// Commit records the current index as a new commit.
//
// The author comes from the repository configuration, falling back to the
// user's global Git configuration.
func (s *Store) Commit(message string) (plumbing.Hash, error) {
	if strings.TrimSpace(message) == "" {
		return plumbing.ZeroHash, errors.New("commit message must not be empty")
	}

	configuration, err := s.repo.ConfigScoped(config.GlobalScope)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf(
			"read repository configuration: %w",
			err,
		)
	}

	if configuration.User.Name == "" {
		return plumbing.ZeroHash, errors.New(
			"user.name is not configured; run " +
				"`dot config user.name \"Your Name\"`",
		)
	}

	if configuration.User.Email == "" {
		return plumbing.ZeroHash, errors.New(
			"user.email is not configured; run " +
				"`dot config user.email \"you@example.com\"`",
		)
	}

	staged, err := s.HasStagedChanges()
	if err != nil {
		return plumbing.ZeroHash, err
	}

	if !staged {
		return plumbing.ZeroHash, ErrNoStagedChanges
	}

	worktree, err := s.repo.Worktree()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("open worktree: %w", err)
	}

	signature := object.Signature{
		Name:  configuration.User.Name,
		Email: configuration.User.Email,
		When:  time.Now(),
	}

	hash, err := worktree.Commit(message, &git.CommitOptions{
		Author:    &signature,
		Committer: &signature,
	})
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("create commit: %w", err)
	}

	return hash, nil
}

// HasStagedChanges reports whether the index differs from HEAD. Unlike
// Status it does not read the working tree.
func (s *Store) HasStagedChanges() (bool, error) {
	idx, err := s.loadIndex()
	if err != nil {
		return false, err
	}

	head, err := s.headFiles()
	if err != nil {
		return false, err
	}

	return !maps.Equal(head, indexFiles(idx)), nil
}
