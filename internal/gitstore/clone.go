package gitstore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	git "github.com/go-git/go-git/v5"
)

var ErrRepositoryPathNotEmpty = errors.New(
	"repository destination is not empty",
)

// Clone clones a Git repository into repositoryPath.
//
// Existing non-empty destinations are rejected. Cloning only populates the
// internal repository and never deploys managed files to the user's home
// directory.
func Clone(
	repositoryPath string,
	remoteURL string,
	progress io.Writer,
) error {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return fmt.Errorf("remote URL must not be empty")
	}

	if err := requireEmptyDestination(repositoryPath); err != nil {
		return err
	}

	auth, err := authenticationForURL(remoteURL)
	if err != nil {
		return fmt.Errorf(
			"configure authentication for remote: %w",
			err,
		)
	}

	_, err = git.PlainClone(
		repositoryPath,
		false,
		&git.CloneOptions{
			URL:        remoteURL,
			RemoteName: "origin",
			Auth:       auth,
			Progress:   progress,
		},
	)
	if err != nil {
		return fmt.Errorf(
			"clone %q into %q: %w",
			remoteURL,
			repositoryPath,
			err,
		)
	}

	return nil
}

func requireEmptyDestination(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf(
			"inspect repository destination %q: %w",
			path,
			err,
		)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf(
			"repository destination %q must not be a symbolic link",
			path,
		)
	}

	if !info.IsDir() {
		return fmt.Errorf(
			"repository destination %q is not a directory",
			path,
		)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf(
			"read repository destination %q: %w",
			path,
			err,
		)
	}

	if len(entries) != 0 {
		return fmt.Errorf(
			"%w: %q",
			ErrRepositoryPathNotEmpty,
			path,
		)
	}

	return nil
}
