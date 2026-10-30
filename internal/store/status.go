package store

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
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
	head, err := s.repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return map[string]treeEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve HEAD: %w", err)
	}

	return s.commitFiles(head.Hash())
}

// commitFiles lists the files of commit hash.
func (s *Store) commitFiles(hash plumbing.Hash) (map[string]treeEntry, error) {
	commit, err := s.repo.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("load commit %s: %w", hash, err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("load tree of commit %s: %w", hash, err)
	}

	files := map[string]treeEntry{}

	err = tree.Files().ForEach(func(file *object.File) error {
		files[file.Name] = treeEntry{hash: file.Hash, mode: file.Mode}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list files of commit %s: %w", hash, err)
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
// Untracked files are never looked at, and neither are variants for other
// machines, which have no file on disk here.
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
			states[name] = &FileState{Name: name, Staged: Unmodified, Worktree: Unmodified}
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

	for filePath, shown := range s.facts.show(staged) {
		file, err := s.readWorktree(filePath, shown.mode)

		switch {
		case errors.Is(err, os.ErrNotExist):
			state(shown.name).Worktree = Deleted
		case err != nil:
			return nil, fmt.Errorf("read %q: %w", filePath, err)
		case file.hash() != shown.hash || file.mode != shown.mode:
			state(shown.name).Worktree = Modified
		}
	}

	result := make([]FileState, 0, len(states))
	for _, fileState := range states {
		result = append(result, *fileState)
	}

	slices.SortFunc(result, func(a, b FileState) int { return strings.Compare(a.Name, b.Name) })

	return result, nil
}

// FileDiff is the difference of one file between two states.
type FileDiff struct {
	Name   string // the tracked name
	Path   string // the path on disk
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

	if staged {
		return s.stagedDiff(idx)
	}

	shown := s.facts.show(indexFiles(idx))
	var diffs []FileDiff

	for _, filePath := range slices.Sorted(maps.Keys(shown)) {
		entry := shown[filePath]

		before, err := s.blobContents(entry.hash)
		if err != nil {
			return nil, err
		}

		file, err := s.readWorktree(filePath, entry.mode)
		exists := err == nil

		switch {
		case errors.Is(err, os.ErrNotExist):
		case err != nil:
			return nil, fmt.Errorf("read %q: %w", filePath, err)
		case file.hash() == entry.hash:
			continue
		}

		var after []byte
		if exists {
			after = file.data
		}

		diffs = append(diffs, buildDiff(entry.name, filePath, before, true, after, exists))
	}

	return diffs, nil
}

// stagedDiff compares HEAD with the index.
func (s *Store) stagedDiff(idx *index.Index) ([]FileDiff, error) {
	head, err := s.headFiles()
	if err != nil {
		return nil, err
	}

	staged := indexFiles(idx)

	names := slices.Collect(maps.Keys(head))
	for name := range staged {
		if _, inHead := head[name]; !inHead {
			names = append(names, name)
		}
	}
	slices.Sort(names)

	var diffs []FileDiff

	for _, name := range names {
		if head[name] == staged[name] {
			continue
		}

		before, existedBefore, err := s.entryContents(head, name)
		if err != nil {
			return nil, err
		}

		after, existsAfter, err := s.entryContents(staged, name)
		if err != nil {
			return nil, err
		}

		filePath, _ := s.facts.SplitName(name)
		diffs = append(diffs, buildDiff(name, filePath, before, existedBefore, after, existsAfter))
	}

	return diffs, nil
}

func (s *Store) entryContents(files map[string]treeEntry, name string) ([]byte, bool, error) {
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

func buildDiff(
	name string,
	filePath string,
	before []byte, existedBefore bool,
	after []byte, existsAfter bool,
) FileDiff {
	diff := FileDiff{Name: name, Path: filePath}

	if isBinary(before) || isBinary(after) {
		diff.Binary = true
		return diff
	}

	fromFile, toFile := "a/"+filePath, "b/"+filePath
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
		patch = fmt.Sprintf("cannot diff %s: %v\n", filePath, err)
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
