package store

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
)

// ErrMergeConflicts is wrapped by MergeConflictError.
var ErrMergeConflicts = errors.New(
	"local and remote commits changed the same files differently",
)

// MergeConflictError lists files that both sides of a merge changed in
// different ways. dot does not merge file contents.
type MergeConflictError struct {
	Paths []string
}

func (e *MergeConflictError) Error() string {
	return fmt.Sprintf("%v: %s", ErrMergeConflicts, strings.Join(e.Paths, ", "))
}

func (e *MergeConflictError) Unwrap() error { return ErrMergeConflicts }

// fileVersion is a file in one commit, which may lack the file.
type fileVersion struct {
	entry  treeEntry
	exists bool
}

func versionOf(files map[string]treeEntry, name string) fileVersion {
	entry, exists := files[name]
	return fileVersion{entry: entry, exists: exists}
}

// mergeFiles merges the files of two commits that descend from base. A file
// changed on one side only takes that side; a file changed identically on
// both sides stays as it is. Files changed differently on both sides are
// returned as conflicts and left out of the result.
func mergeFiles(
	base map[string]treeEntry,
	local map[string]treeEntry,
	remote map[string]treeEntry,
) (map[string]treeEntry, []string) {
	names := map[string]bool{}
	for _, files := range []map[string]treeEntry{base, local, remote} {
		for name := range files {
			names[name] = true
		}
	}

	merged := map[string]treeEntry{}
	var conflicts []string

	for name := range names {
		original := versionOf(base, name)
		ours := versionOf(local, name)
		theirs := versionOf(remote, name)

		var chosen fileVersion

		switch {
		case ours == theirs, theirs == original:
			chosen = ours
		case ours == original:
			chosen = theirs
		default:
			conflicts = append(conflicts, name)
			continue
		}

		if chosen.exists {
			merged[name] = chosen.entry
		}
	}

	slices.Sort(conflicts)

	return merged, conflicts
}

// mergeBase returns the best common ancestor of two commits.
func (s *Store) mergeBase(a plumbing.Hash, b plumbing.Hash) (plumbing.Hash, error) {
	first, err := s.repo.CommitObject(a)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("load commit %s: %w", a, err)
	}

	second, err := s.repo.CommitObject(b)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("load commit %s: %w", b, err)
	}

	bases, err := first.MergeBase(second)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("find common ancestor: %w", err)
	}

	if len(bases) == 0 {
		return plumbing.ZeroHash, errors.New(
			"local and remote branch share no history",
		)
	}

	return bases[0].Hash, nil
}

// mergedFiles merges the commits local and remote and lists the resulting
// files, or returns a MergeConflictError.
func (s *Store) mergedFiles(
	local plumbing.Hash,
	remote plumbing.Hash,
) (map[string]treeEntry, error) {
	baseHash, err := s.mergeBase(local, remote)
	if err != nil {
		return nil, err
	}

	var files [3]map[string]treeEntry

	for i, hash := range []plumbing.Hash{baseHash, local, remote} {
		files[i], err = s.commitFiles(hash)
		if err != nil {
			return nil, err
		}
	}

	merged, conflicts := mergeFiles(files[0], files[1], files[2])
	if len(conflicts) > 0 {
		return nil, &MergeConflictError{Paths: conflicts}
	}

	return merged, nil
}
