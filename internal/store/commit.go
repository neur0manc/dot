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
	staged, err := s.HasStagedChanges()
	if err != nil {
		return plumbing.ZeroHash, err
	}

	if !staged {
		return plumbing.ZeroHash, ErrNoStagedChanges
	}

	return s.commit(message, nil)
}

// commit records the index as a commit on top of parents, which default to
// HEAD. Several parents make a merge commit, which may leave the tree as it is.
func (s *Store) commit(
	message string,
	parents []plumbing.Hash,
) (plumbing.Hash, error) {
	if strings.TrimSpace(message) == "" {
		return plumbing.ZeroHash, errors.New("commit message must not be empty")
	}

	signature, err := s.signature()
	if err != nil {
		return plumbing.ZeroHash, err
	}

	worktree, err := s.repo.Worktree()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("open worktree: %w", err)
	}

	hash, err := worktree.Commit(message, &git.CommitOptions{
		Author:            &signature,
		Committer:         &signature,
		Parents:           parents,
		AllowEmptyCommits: len(parents) > 0,
	})
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("create commit: %w", err)
	}

	return hash, nil
}

// signature identifies the committer by the configured user name and email.
func (s *Store) signature() (object.Signature, error) {
	configuration, err := s.repo.ConfigScoped(config.GlobalScope)
	if err != nil {
		return object.Signature{}, fmt.Errorf(
			"read repository configuration: %w",
			err,
		)
	}

	if configuration.User.Name == "" {
		return object.Signature{}, errors.New(
			"user.name is not configured; run " +
				"`dot config user.name \"Your Name\"`",
		)
	}

	if configuration.User.Email == "" {
		return object.Signature{}, errors.New(
			"user.email is not configured; run " +
				"`dot config user.email \"you@example.com\"`",
		)
	}

	return object.Signature{
		Name:  configuration.User.Name,
		Email: configuration.User.Email,
		When:  time.Now(),
	}, nil
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
