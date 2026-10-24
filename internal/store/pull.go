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

	// ErrPullNonFastForward means the histories have diverged.
	ErrPullNonFastForward = errors.New(
		"pull would require a non-fast-forward update",
	)
)

// Pull fetches the current branch from remoteName, fast-forwards the local
// branch and updates the tracked files in the working tree.
//
// Tracked files with local modifications that the update would overwrite are
// reported as a ConflictError unless force is set. The returned Boolean is
// true when the branch moved.
func (s *Store) Pull(
	remoteName string,
	force bool,
	progress io.Writer,
) (bool, error) {
	remoteName = strings.TrimSpace(remoteName)
	if remoteName == "" {
		return false, errors.New("remote name must not be empty")
	}

	staged, err := s.HasStagedChanges()
	if err != nil {
		return false, err
	}

	if staged {
		return false, ErrPullStagedChanges
	}

	head, err := s.repo.Head()
	if err != nil {
		return false, fmt.Errorf("resolve current branch: %w", err)
	}

	branch := head.Name()
	if !branch.IsBranch() {
		return false, fmt.Errorf(
			"cannot pull while HEAD is detached at %s",
			head.Hash(),
		)
	}

	remote, err := s.repo.Remote(remoteName)
	if err != nil {
		return false, fmt.Errorf("open remote %q: %w", remoteName, err)
	}

	urls := remote.Config().URLs
	if len(urls) == 0 {
		return false, fmt.Errorf("remote %q has no URL", remoteName)
	}

	auth, err := authenticationForURL(urls[0])
	if err != nil {
		return false, fmt.Errorf(
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
		return false, fmt.Errorf(
			"fetch branch %q from remote %q: %w",
			branch.Short(),
			remoteName,
			err,
		)
	}

	fetched, err := s.repo.Reference(remoteReference, true)
	if err != nil {
		return false, fmt.Errorf(
			"resolve fetched branch %q from remote %q: %w",
			branch.Short(),
			remoteName,
			err,
		)
	}

	localHash, remoteHash := head.Hash(), fetched.Hash()
	if localHash == remoteHash {
		return false, nil
	}

	fastForward, err := s.isAncestor(localHash, remoteHash)
	if err != nil {
		return false, fmt.Errorf(
			"check whether branch %q can be fast-forwarded: %w",
			branch.Short(),
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

	current, err := s.commitFiles(localHash)
	if err != nil {
		return false, err
	}

	target, err := s.commitFiles(remoteHash)
	if err != nil {
		return false, err
	}

	if _, err := s.update(current, target, false, force); err != nil {
		return false, err
	}

	reference := plumbing.NewHashReference(branch, remoteHash)
	if err := s.repo.Storer.SetReference(reference); err != nil {
		return false, fmt.Errorf("update branch %q: %w", branch.Short(), err)
	}

	return true, nil
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
