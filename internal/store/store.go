// Package store keeps a user's configuration files in a Git repository whose
// working tree is the user's home directory, in the style of yadm.
//
// The Git directory lives elsewhere (see the paths package). Only files that
// were explicitly added are tracked, so the home directory is never scanned
// for untracked files.
package store

import (
	"errors"
	"fmt"
	"os"

	"github.com/go-git/go-billy/v5/osfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// DefaultBranch is the branch created by Init.
const DefaultBranch = "main"

var (
	// ErrNotInitialized means no dotmgr repository exists yet.
	ErrNotInitialized = errors.New(
		"no dotmgr repository found; run `dotmgr init` or `dotmgr clone <url>`",
	)

	// ErrAlreadyExists means a repository is already present.
	ErrAlreadyExists = errors.New("repository already exists")
)

// Store is a Git repository with the home directory as its working tree.
type Store struct {
	repo   *git.Repository
	gitDir string
	home   string
	facts  Facts // the machine that variants are chosen for
}

func newStorage(gitDir string) *filesystem.Storage {
	return filesystem.NewStorage(
		osfs.New(gitDir),
		cache.NewObjectLRUDefault(),
	)
}

// Open opens the repository stored in gitDir with home as working tree.
func Open(gitDir string, home string) (*Store, error) {
	repo, err := git.Open(newStorage(gitDir), osfs.New(home))
	if errors.Is(err, git.ErrRepositoryNotExists) {
		return nil, ErrNotInitialized
	}
	if err != nil {
		return nil, fmt.Errorf("open repository at %q: %w", gitDir, err)
	}

	s := &Store{repo: repo, gitDir: gitDir, home: home}
	if err := s.loadFacts(); err != nil {
		return nil, err
	}

	return s, nil
}

// Init creates a new, empty repository in gitDir.
func Init(gitDir string, home string) (*Store, error) {
	if err := requireEmptyDestination(gitDir); err != nil {
		return nil, err
	}

	if err := createBare(gitDir); err != nil {
		return nil, err
	}

	return Open(gitDir, home)
}

// createBare initializes gitDir and marks it as having a working tree.
//
// go-git writes a ".git" file into the working tree when it is initialized
// together with one, which must never happen in a home directory. The
// repository is therefore created without a working tree and flagged as
// non-bare afterwards.
func createBare(gitDir string) error {
	repo, err := git.InitWithOptions(
		newStorage(gitDir),
		nil,
		git.InitOptions{
			DefaultBranch: plumbing.NewBranchReferenceName(DefaultBranch),
		},
	)
	if err != nil {
		return fmt.Errorf("initialize Git repository: %w", err)
	}

	return markNonBare(repo)
}

func markNonBare(repo *git.Repository) error {
	configuration, err := repo.Config()
	if err != nil {
		return fmt.Errorf("read repository configuration: %w", err)
	}

	configuration.Core.IsBare = false

	if err := repo.SetConfig(configuration); err != nil {
		return fmt.Errorf("write repository configuration: %w", err)
	}

	return nil
}

// Head returns the current commit.
func (s *Store) Head() (plumbing.Hash, error) {
	head, err := s.repo.Head()
	if err != nil {
		return plumbing.ZeroHash, err
	}

	return head.Hash(), nil
}

// GitDir returns the directory holding the Git repository.
func (s *Store) GitDir() string { return s.gitDir }

// Home returns the working tree directory.
func (s *Store) Home() string { return s.home }

func requireEmptyDestination(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect repository destination %q: %w", path, err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf(
			"repository destination %q must not be a symbolic link",
			path,
		)
	}

	if !info.IsDir() {
		return fmt.Errorf("repository destination %q is not a directory", path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("read repository destination %q: %w", path, err)
	}

	if len(entries) != 0 {
		return fmt.Errorf("%w: %q", ErrAlreadyExists, path)
	}

	return nil
}
