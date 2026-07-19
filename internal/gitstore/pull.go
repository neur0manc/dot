package gitstore

import (
	"errors"
	"fmt"
	"io"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

var (
	ErrPullDirtyRepository = errors.New(
		"repository contains uncommitted changes",
	)

	ErrPullNonFastForward = errors.New(
		"pull would require a non-fast-forward update",
	)
)

// Pull fetches the current branch from remoteName and updates the local
// branch only when the update is a fast-forward.
//
// The returned Boolean is true when the local branch was updated and false
// when it was already up to date.
//
// Pull never applies managed files to their host targets.
func Pull(
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

	worktree, err := repository.Worktree()
	if err != nil {
		return false, fmt.Errorf(
			"open repository worktree: %w",
			err,
		)
	}

	status, err := worktree.Status()
	if err != nil {
		return false, fmt.Errorf(
			"read repository status: %w",
			err,
		)
	}

	if !status.IsClean() {
		return false, fmt.Errorf(
			"%w; commit or discard them before pulling",
			ErrPullDirtyRepository,
		)
	}

	head, err := repository.Head()
	if err != nil {
		return false, fmt.Errorf(
			"resolve current branch: %w",
			err,
		)
	}

	branchReference := head.Name()
	if !branchReference.IsBranch() {
		return false, fmt.Errorf(
			"cannot pull while HEAD is detached at %s",
			head.Hash(),
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

	remoteReference := plumbing.NewRemoteReferenceName(
		remoteName,
		branchReference.Short(),
	)

	refspec := config.RefSpec(
		fmt.Sprintf(
			"+%s:%s",
			branchReference,
			remoteReference,
		),
	)

	err = repository.Fetch(&git.FetchOptions{
		RemoteName: remoteName,
		RefSpecs:   []config.RefSpec{refspec},
		Auth:       auth,
		Progress:   progress,
		Force:      true,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return false, fmt.Errorf(
			"fetch branch %q from remote %q: %w",
			branchReference.Short(),
			remoteName,
			err,
		)
	}

	fetchedReference, err := repository.Reference(
		remoteReference,
		true,
	)
	if err != nil {
		return false, fmt.Errorf(
			"resolve fetched branch %q from remote %q: %w",
			branchReference.Short(),
			remoteName,
			err,
		)
	}

	localHash := head.Hash()
	remoteHash := fetchedReference.Hash()

	if localHash == remoteHash {
		return false, nil
	}

	fastForward, err := isCommitAncestor(
		repository,
		localHash,
		remoteHash,
	)
	if err != nil {
		return false, fmt.Errorf(
			"check whether branch %q can be fast-forwarded: %w",
			branchReference.Short(),
			err,
		)
	}

	if !fastForward {
		return false, fmt.Errorf(
			"%w: local %s, remote %s",
			ErrPullNonFastForward,
			localHash,
			remoteHash,
		)
	}

	if err := worktree.Reset(&git.ResetOptions{
		Commit: remoteHash,
		Mode:   git.HardReset,
	}); err != nil {
		return false, fmt.Errorf(
			"fast-forward branch %q to %s: %w",
			branchReference.Short(),
			remoteHash,
			err,
		)
	}

	return true, nil
}

// isCommitAncestor reports whether ancestorHash is reachable from
// descendantHash by following parent commits.
func isCommitAncestor(
	repository *git.Repository,
	ancestorHash plumbing.Hash,
	descendantHash plumbing.Hash,
) (bool, error) {
	pending := []plumbing.Hash{descendantHash}
	visited := make(map[plumbing.Hash]struct{})

	for len(pending) > 0 {
		lastIndex := len(pending) - 1
		currentHash := pending[lastIndex]
		pending = pending[:lastIndex]

		if currentHash == ancestorHash {
			return true, nil
		}

		if _, alreadyVisited := visited[currentHash]; alreadyVisited {
			continue
		}

		visited[currentHash] = struct{}{}

		commit, err := repository.CommitObject(currentHash)
		if err != nil {
			return false, fmt.Errorf(
				"load commit %s: %w",
				currentHash,
				err,
			)
		}

		pending = append(pending, commit.ParentHashes...)
	}

	return false, nil
}
