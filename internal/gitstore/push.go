package gitstore

import (
	"fmt"
	"io"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// Push pushes the current branch to the remote with the same branch name.
//
// The returned Boolean is true when data was pushed and false when the remote
// was already up to date.
func Push(
	repositoryPath string,
	remoteName string,
	progress io.Writer,
) (bool, error) {
	remoteName = strings.TrimSpace(remoteName)
	if remoteName == "" {
		return false, fmt.Errorf("remote name must not be empty")
	}

	repository, err := git.PlainOpen(repositoryPath)
	if err != nil {
		return false, fmt.Errorf(
			"open repository at %q: %w",
			repositoryPath,
			err,
		)
	}

	remote, err := repository.Remote(remoteName)
	if err != nil {
		return false, fmt.Errorf(
			"open remote %q: %w",
			remoteName,
			err,
		)
	}

	remoteConfiguration := remote.Config()
	if len(remoteConfiguration.URLs) == 0 {
		return false, fmt.Errorf(
			"remote %q has no URL",
			remoteName,
		)
	}

	head, err := repository.Head()
	if err != nil {
		return false, fmt.Errorf(
			"resolve current branch: %w",
			err,
		)
	}

	branchName := head.Name()
	if !branchName.IsBranch() {
		return false, fmt.Errorf(
			"cannot push detached HEAD %s",
			head.Hash(),
		)
	}

	auth, err := authenticationForURL(
		remoteConfiguration.URLs[0],
	)
	if err != nil {
		return false, fmt.Errorf(
			"configure authentication for remote %q: %w",
			remoteName,
			err,
		)
	}

	refspec := config.RefSpec(
		fmt.Sprintf("%s:%s", branchName, branchName),
	)

	err = repository.Push(&git.PushOptions{
		RemoteName: remoteName,
		RefSpecs:   []config.RefSpec{refspec},
		Auth:       auth,
		Progress:   progress,
		Force:      false,
	})
	if err == git.NoErrAlreadyUpToDate {
		if err := setBranchUpstream(
			repository,
			branchName,
			remoteName,
		); err != nil {
			return false, err
		}

		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf(
			"push branch %q to remote %q: %w",
			branchName.Short(),
			remoteName,
			err,
		)
	}

	if err := setBranchUpstream(
		repository,
		branchName,
		remoteName,
	); err != nil {
		return false, err
	}

	return true, nil
}

func setBranchUpstream(
	repository *git.Repository,
	branchName plumbing.ReferenceName,
	remoteName string,
) error {
	configuration, err := repository.Config()
	if err != nil {
		return fmt.Errorf(
			"read repository configuration: %w",
			err,
		)
	}

	if configuration.Branches == nil {
		configuration.Branches = map[string]*config.Branch{}
	}

	shortName := branchName.Short()

	configuration.Branches[shortName] = &config.Branch{
		Name:   shortName,
		Remote: remoteName,
		Merge:  branchName,
	}

	if err := repository.Storer.SetConfig(configuration); err != nil {
		return fmt.Errorf(
			"store upstream configuration for branch %q: %w",
			shortName,
			err,
		)
	}

	return nil
}
