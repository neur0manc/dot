package store

import (
	"errors"
	"fmt"
	"io"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// Push pushes the current branch to the same-named branch of remoteName.
//
// The first remote a branch is pushed to becomes its upstream; SetUpstream
// changes it later.
//
// The returned Boolean is true when data was pushed and false when the remote
// was already up to date.
func (s *Store) Push(remoteName string, progress io.Writer) (bool, error) {
	remoteName = strings.TrimSpace(remoteName)
	if remoteName == "" {
		return false, errors.New("remote name must not be empty")
	}

	remote, err := s.repo.Remote(remoteName)
	if err != nil {
		return false, fmt.Errorf("open remote %q: %w", remoteName, err)
	}

	urls := remote.Config().URLs
	if len(urls) == 0 {
		return false, fmt.Errorf("remote %q has no URL", remoteName)
	}

	head, err := s.repo.Head()
	if err != nil {
		return false, fmt.Errorf("resolve current branch: %w", err)
	}

	branch := head.Name()
	if !branch.IsBranch() {
		return false, fmt.Errorf("cannot push detached HEAD %s", head.Hash())
	}

	auth, err := authenticationForURL(urls[0])
	if err != nil {
		return false, fmt.Errorf(
			"configure authentication for remote %q: %w",
			remoteName,
			err,
		)
	}

	err = s.repo.Push(&git.PushOptions{
		RemoteName: remoteName,
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("%s:%s", branch, branch)),
		},
		Auth:     auth,
		Progress: progress,
	})

	pushed := true

	switch {
	case errors.Is(err, git.NoErrAlreadyUpToDate):
		pushed = false
	case err != nil:
		return false, fmt.Errorf(
			"push branch %q to remote %q: %w",
			branch.Short(),
			remoteName,
			err,
		)
	}

	// A later push to another remote must not silently change the default.
	current, err := s.upstream(branch)
	if err != nil {
		return false, err
	}

	if current == "" {
		if err := s.setUpstream(branch, remoteName); err != nil {
			return false, err
		}
	}

	return pushed, nil
}

func (s *Store) setUpstream(
	branch plumbing.ReferenceName,
	remoteName string,
) error {
	configuration, err := s.repo.Config()
	if err != nil {
		return fmt.Errorf("read repository configuration: %w", err)
	}

	if configuration.Branches == nil {
		configuration.Branches = map[string]*config.Branch{}
	}

	configuration.Branches[branch.Short()] = &config.Branch{
		Name:   branch.Short(),
		Remote: remoteName,
		Merge:  branch,
	}

	if err := s.repo.SetConfig(configuration); err != nil {
		return fmt.Errorf(
			"store upstream configuration for branch %q: %w",
			branch.Short(),
			err,
		)
	}

	return nil
}
