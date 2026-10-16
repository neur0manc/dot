package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	git "github.com/go-git/go-git/v5"
)

// Clone clones remoteURL into gitDir and checks the tracked files out into
// home.
//
// gitDir must not exist or be empty. When existing files in home differ from
// the repository, the clone is kept and the returned Store is non-nil
// together with a ConflictError; nothing in home has been changed then, and
// Checkout with force resolves it.
func Clone(
	gitDir string,
	home string,
	remoteURL string,
	progress io.Writer,
) (*Store, SyncResult, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return nil, SyncResult{}, errors.New("remote URL must not be empty")
	}

	if err := requireEmptyDestination(gitDir); err != nil {
		return nil, SyncResult{}, err
	}

	auth, err := authenticationForURL(remoteURL)
	if err != nil {
		return nil, SyncResult{}, fmt.Errorf(
			"configure authentication for remote: %w",
			err,
		)
	}

	repo, err := git.Clone(newStorage(gitDir), nil, &git.CloneOptions{
		URL:        remoteURL,
		RemoteName: "origin",
		Auth:       auth,
		Progress:   progress,
	})
	if err != nil {
		os.RemoveAll(gitDir)
		return nil, SyncResult{}, fmt.Errorf("clone %q: %w", remoteURL, err)
	}

	if err := markNonBare(repo); err != nil {
		return nil, SyncResult{}, err
	}

	store, err := Open(gitDir, home)
	if err != nil {
		return nil, SyncResult{}, err
	}

	// An empty remote has no HEAD commit and nothing to check out.
	if _, err := store.repo.Head(); err != nil {
		return store, SyncResult{}, nil
	}

	result, err := store.Checkout(false)

	return store, result, err
}
