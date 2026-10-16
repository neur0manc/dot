package store

import (
	"errors"
	"fmt"
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
				"`dotmgr config user.name \"Your Name\"`",
		)
	}

	if configuration.User.Email == "" {
		return plumbing.ZeroHash, errors.New(
			"user.email is not configured; run " +
				"`dotmgr config user.email \"you@example.com\"`",
		)
	}

	states, err := s.Status()
	if err != nil {
		return plumbing.ZeroHash, err
	}

	if !hasStaged(states) {
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

func hasStaged(states []FileState) bool {
	for _, state := range states {
		if state.Staged != Unmodified {
			return true
		}
	}

	return false
}

// SetUserConfig writes the repository-local commit identity.
func (s *Store) SetUserConfig(key string, value string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", key)
	}

	configuration, err := s.repo.Config()
	if err != nil {
		return fmt.Errorf("read repository configuration: %w", err)
	}

	switch key {
	case "user.name":
		configuration.User.Name = value
	case "user.email":
		configuration.User.Email = value
	default:
		return fmt.Errorf("unsupported configuration key %q", key)
	}

	if err := s.repo.SetConfig(configuration); err != nil {
		return fmt.Errorf("write repository configuration: %w", err)
	}

	return nil
}

// UserConfig reads the commit identity as Git would resolve it.
func (s *Store) UserConfig(key string) (string, error) {
	configuration, err := s.repo.ConfigScoped(config.GlobalScope)
	if err != nil {
		return "", fmt.Errorf("read repository configuration: %w", err)
	}

	switch key {
	case "user.name":
		return configuration.User.Name, nil
	case "user.email":
		return configuration.User.Email, nil
	default:
		return "", fmt.Errorf("unsupported configuration key %q", key)
	}
}
