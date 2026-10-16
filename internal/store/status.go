package store

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/pmezard/go-difflib/difflib"
)

// Code describes how a file differs, using Git's short status letters.
type Code byte

const (
	Unmodified Code = ' '
	Added      Code = 'A'
	Modified   Code = 'M'
	Deleted    Code = 'D'
)

// FileState compares one file between HEAD, the index and the working tree.
type FileState struct {
	Name     string
	Staged   Code // HEAD compared with the index
	Worktree Code // index compared with the working tree
}

func (f FileState) String() string {
	return fmt.Sprintf("%c%c %s", f.Staged, f.Worktree, f.Name)
}

type treeEntry struct {
	hash plumbing.Hash
	mode filemode.FileMode
}

// headFiles lists the files of the HEAD commit; an unborn branch has none.
func (s *Store) headFiles() (map[string]treeEntry, error) {
	files := map[string]treeEntry{}

	head, err := s.repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return files, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve HEAD: %w", err)
	}

	commit, err := s.repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("load HEAD commit: %w", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("load HEAD tree: %w", err)
	}

	err = tree.Files().ForEach(func(file *object.File) error {
		files[file.Name] = treeEntry{hash: file.Hash, mode: file.Mode}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list HEAD files: %w", err)
	}

	return files, nil
}

func indexFiles(idx *index.Index) map[string]treeEntry {
	files := make(map[string]treeEntry, len(idx.Entries))

	for _, entry := range idx.Entries {
		files[entry.Name] = treeEntry{hash: entry.Hash, mode: entry.Mode}
	}

	return files
}

// Status reports every tracked file that differs from HEAD or the index.
// Untracked files are never looked at.
func (s *Store) Status() ([]FileState, error) {
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	head, err := s.headFiles()
	if err != nil {
		return nil, err
	}

	staged := indexFiles(idx)
	states := map[string]*FileState{}

	state := func(name string) *FileState {
		if states[name] == nil {
			states[name] = &FileState{
				Name:     name,
				Staged:   Unmodified,
				Worktree: Unmodified,
			}
		}

		return states[name]
	}

	for name, entry := range staged {
		previous, inHead := head[name]

		switch {
		case !inHead:
			state(name).Staged = Added
		case previous != entry:
			state(name).Staged = Modified
		}
	}

	for name := range head {
		if _, inIndex := staged[name]; !inIndex {
			state(name).Staged = Deleted
		}
	}

	for name, entry := range staged {
		file, err := s.readWorktree(name, entry.mode)

		switch {
		case errors.Is(err, os.ErrNotExist):
			state(name).Worktree = Deleted
		case err != nil:
			return nil, fmt.Errorf("read %q: %w", name, err)
		case file.hash() != entry.hash || file.mode != entry.mode:
			state(name).Worktree = Modified
		}
	}

	result := make([]FileState, 0, len(states))

	for _, fileState := range states {
		result = append(result, *fileState)
	}

	sort.Slice(result, func(left int, right int) bool {
		return result[left].Name < result[right].Name
	})

	return result, nil
}

// FileDiff is the difference of one file between two states.
type FileDiff struct {
	Name   string
	Binary bool
	Patch  string
}

// Diff compares the index with the working tree, or with HEAD when staged is
// true. Only tracked files that differ are returned.
func (s *Store) Diff(staged bool) ([]FileDiff, error) {
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	var oldFiles, newFiles map[string]treeEntry

	if staged {
		oldFiles, err = s.headFiles()
		if err != nil {
			return nil, err
		}

		newFiles = indexFiles(idx)
	} else {
		oldFiles = indexFiles(idx)
	}

	names := map[string]struct{}{}
	for name := range oldFiles {
		names[name] = struct{}{}
	}
	for name := range newFiles {
		names[name] = struct{}{}
	}

	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	var diffs []FileDiff

	for _, name := range sorted {
		before, existedBefore, err := s.sideContents(oldFiles, name)
		if err != nil {
			return nil, err
		}

		var after []byte
		var existsAfter bool

		if staged {
			after, existsAfter, err = s.sideContents(newFiles, name)
		} else {
			after, existsAfter, err = s.worktreeContents(oldFiles, name)
		}
		if err != nil {
			return nil, err
		}

		if existedBefore == existsAfter && string(before) == string(after) {
			continue
		}

		diffs = append(diffs, buildDiff(
			name,
			before, existedBefore,
			after, existsAfter,
		))
	}

	return diffs, nil
}

func (s *Store) sideContents(
	files map[string]treeEntry,
	name string,
) ([]byte, bool, error) {
	entry, ok := files[name]
	if !ok {
		return nil, false, nil
	}

	data, err := s.blobContents(entry.hash)
	if err != nil {
		return nil, false, err
	}

	return data, true, nil
}

func (s *Store) worktreeContents(
	reference map[string]treeEntry,
	name string,
) ([]byte, bool, error) {
	file, err := s.readWorktree(name, reference[name].mode)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %q: %w", name, err)
	}

	return file.data, true, nil
}

func buildDiff(
	name string,
	before []byte, existedBefore bool,
	after []byte, existsAfter bool,
) FileDiff {
	diff := FileDiff{Name: name}

	if isBinary(before) || isBinary(after) {
		diff.Binary = true
		return diff
	}

	fromFile, toFile := "a/"+name, "b/"+name
	if !existedBefore {
		fromFile = "/dev/null"
	}
	if !existsAfter {
		toFile = "/dev/null"
	}

	patch, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(before)),
		B:        difflib.SplitLines(string(after)),
		FromFile: fromFile,
		ToFile:   toFile,
		Context:  3,
	})
	if err != nil {
		patch = fmt.Sprintf("cannot diff %s: %v\n", name, err)
	}

	diff.Patch = ensureTrailingNewline(patch)

	return diff
}

func ensureTrailingNewline(text string) string {
	if text != "" && !strings.HasSuffix(text, "\n") {
		return text + "\n"
	}

	return text
}
