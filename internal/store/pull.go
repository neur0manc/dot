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

var (
	// ErrPullStagedChanges means staged changes block a pull.
	ErrPullStagedChanges = errors.New(
		"there are staged changes; commit them before pulling",
	)
)

// PullResult tells what a pull did to the current branch.
type PullResult struct {
	Updated bool // the branch moved
	Merged  bool // local and remote commits were joined by a merge commit
}

// Pull fetches the current branch from remoteName, fast-forwards the local
// branch and updates the tracked files in the working tree.
//
// When local and remote both have new commits, they are joined by a merge
// commit. Files that both sides changed differently are reported as a
// MergeConflictError and nothing is changed.
//
// Tracked files with local modifications that the update would overwrite are
// reported as a ConflictError unless force is set.
func (s *Store) Pull(
	remoteName string,
	force bool,
	progress io.Writer,
) (PullResult, error) {
	remoteName = strings.TrimSpace(remoteName)
	if remoteName == "" {
		return PullResult{}, errors.New("remote name must not be empty")
	}

	staged, err := s.HasStagedChanges()
	if err != nil {
		return PullResult{}, err
	}

	if staged {
		return PullResult{}, ErrPullStagedChanges
	}

	head, err := s.repo.Head()
	if err != nil {
		return PullResult{}, fmt.Errorf("resolve current branch: %w", err)
	}

	branch := head.Name()
	if !branch.IsBranch() {
		return PullResult{}, fmt.Errorf(
			"cannot pull while HEAD is detached at %s",
			head.Hash(),
		)
	}

	remote, err := s.repo.Remote(remoteName)
	if err != nil {
		return PullResult{}, fmt.Errorf("open remote %q: %w", remoteName, err)
	}

	urls := remote.Config().URLs
	if len(urls) == 0 {
		return PullResult{}, fmt.Errorf("remote %q has no URL", remoteName)
	}

	auth, err := authenticationForURL(urls[0])
	if err != nil {
		return PullResult{}, fmt.Errorf(
			"configure authentication for remote %q: %w",
			remoteName,
			err,
		)
	}

	remoteReference := plumbing.NewRemoteReferenceName(
		remoteName,
		branch.Short(),
	)

	err = s.repo.Fetch(&git.FetchOptions{
		RemoteName: remoteName,
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("+%s:%s", branch, remoteReference)),
		},
		Auth:     auth,
		Progress: progress,
		Force:    true,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return PullResult{}, fmt.Errorf(
			"fetch branch %q from remote %q: %w",
			branch.Short(),
			remoteName,
			err,
		)
	}

	fetched, err := s.repo.Reference(remoteReference, true)
	if err != nil {
		return PullResult{}, fmt.Errorf(
			"resolve fetched branch %q from remote %q: %w",
			branch.Short(),
			remoteName,
			err,
		)
	}

	localHash, remoteHash := head.Hash(), fetched.Hash()
	if localHash == remoteHash {
		return PullResult{}, nil
	}

	remoteBehind, err := s.isAncestor(remoteHash, localHash)
	if err != nil {
		return PullResult{}, fmt.Errorf(
			"compare branch %q with the remote: %w",
			branch.Short(),
			err,
		)
	}

	if remoteBehind {
		return PullResult{}, nil
	}

	fastForward, err := s.isAncestor(localHash, remoteHash)
	if err != nil {
		return PullResult{}, fmt.Errorf(
			"check whether branch %q can be fast-forwarded: %w",
			branch.Short(),
			err,
		)
	}

	current, err := s.commitFiles(localHash)
	if err != nil {
		return PullResult{}, err
	}

	if fastForward {
		target, err := s.commitFiles(remoteHash)
		if err != nil {
			return PullResult{}, err
		}

		if _, err := s.update(current, target, false, force); err != nil {
			return PullResult{}, err
		}

		reference := plumbing.NewHashReference(branch, remoteHash)
		if err := s.repo.Storer.SetReference(reference); err != nil {
			return PullResult{}, fmt.Errorf("update branch %q: %w", branch.Short(), err)
		}

		return PullResult{Updated: true}, nil
	}

	message := fmt.Sprintf(
		"Merge %s/%s into %s",
		remoteName,
		branch.Short(),
		branch.Short(),
	)

	if err := s.merge(current, localHash, remoteHash, message, force); err != nil {
		return PullResult{}, err
	}

	return PullResult{Updated: true, Merged: true}, nil
}

// merge joins the commits local and remote: the working tree and the index
// follow the merged files, and a merge commit records them. Everything that
// can fail is checked before the first file is written.
func (s *Store) merge(
	current map[string]treeEntry,
	local plumbing.Hash,
	remote plumbing.Hash,
	message string,
	force bool,
) error {
	if _, err := s.signature(); err != nil {
		return err
	}

	merged, err := s.mergedFiles(local, remote)
	if err != nil {
		return err
	}

	if _, err := s.update(current, merged, false, force); err != nil {
		return err
	}

	_, err = s.commit(message, []plumbing.Hash{local, remote})

	return err
}

// isAncestor reports whether ancestor is reachable from descendant.
func (s *Store) isAncestor(ancestor plumbing.Hash, descendant plumbing.Hash) (bool, error) {
	ancestorCommit, err := s.repo.CommitObject(ancestor)
	if err != nil {
		return false, fmt.Errorf("load commit %s: %w", ancestor, err)
	}

	descendantCommit, err := s.repo.CommitObject(descendant)
	if err != nil {
		return false, fmt.Errorf("load commit %s: %w", descendant, err)
	}

	return ancestorCommit.IsAncestor(descendantCommit)
}

// Checkout writes all files of HEAD to the working tree, restoring deleted
// files and switching files to the variant that fits the machine now.
// Modified files are reported as a ConflictError unless force is set.
func (s *Store) Checkout(force bool) (SyncResult, error) {
	head, err := s.repo.Head()
	if err != nil {
		return SyncResult{}, fmt.Errorf("resolve HEAD: %w", err)
	}

	files, err := s.commitFiles(head.Hash())
	if err != nil {
		return SyncResult{}, err
	}

	return s.update(files, files, true, force)
}
