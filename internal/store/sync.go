package store

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"

	"git.sr.ht/~stephan/dotmgr/internal/atomicfile"
)

// ErrConflicts is wrapped by ConflictError.
var ErrConflicts = errors.New("local files differ from the repository")

// ConflictError lists files that dotmgr refused to overwrite or delete.
type ConflictError struct {
	Paths []string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%v: %s", ErrConflicts, strings.Join(e.Paths, ", "))
}

func (e *ConflictError) Unwrap() error { return ErrConflicts }

// SyncResult summarizes what changed in the working tree.
type SyncResult struct {
	Written []string
	Removed []string
}

// update makes the files on disk and the index follow target. current are
// the files that dotmgr last put on disk, normally those of HEAD.
//
// With restore, every file is checked, so deleted or modified files are put
// back; without it only files that differ between current and target are
// touched, as after a fast-forward.
//
// A file on disk is overwritten or deleted only when it is unmodified: equal
// to one of its variants in current, or already equal to target. Anything
// else is a conflict. Conflicts abort the whole operation before any file is
// written, unless force is set.
//
// Index entries that are in neither current nor target, such as newly added
// files, are kept.
func (s *Store) update(
	current map[string]treeEntry,
	target map[string]treeEntry,
	restore bool,
	force bool,
) (SyncResult, error) {
	var result SyncResult

	for name := range target {
		if err := validateName(name); err != nil {
			return result, err
		}
	}

	wanted := s.facts.show(target)
	present := s.facts.show(current)

	// Every committed content of a path counts as unmodified, so that a file
	// can switch to another variant when the machine's class changes.
	committed := map[string][]plumbing.Hash{}
	for name, entry := range current {
		filePath, _ := SplitName(name)
		committed[filePath] = append(committed[filePath], entry.hash)
	}

	unmodified := func(filePath string, file *worktreeFile) bool {
		return force || slices.Contains(committed[filePath], file.hash())
	}

	var writes, deletes, conflicts []string

	for filePath, entry := range wanted {
		if shown, ok := present[filePath]; ok && shown.treeEntry == entry.treeEntry && !restore {
			continue
		}

		if s.traversesSymlink(filePath, wanted) {
			return result, fmt.Errorf(
				"refusing to write %q through a symbolic link in the repository",
				filePath,
			)
		}

		file, err := s.readWorktree(filePath, entry.mode)

		switch {
		case errors.Is(err, os.ErrNotExist):
			writes = append(writes, filePath)
		case err != nil:
			conflicts = append(conflicts, filePath)
		case file.hash() == entry.hash && file.mode == entry.mode:
			// Already in place.
		case unmodified(filePath, file):
			writes = append(writes, filePath)
		default:
			conflicts = append(conflicts, filePath)
		}
	}

	for filePath, entry := range present {
		if _, kept := wanted[filePath]; kept {
			continue
		}

		file, err := s.readWorktree(filePath, entry.mode)

		switch {
		case errors.Is(err, os.ErrNotExist):
		case err != nil:
			conflicts = append(conflicts, filePath)
		case unmodified(filePath, file):
			deletes = append(deletes, filePath)
		default:
			conflicts = append(conflicts, filePath)
		}
	}

	if len(conflicts) > 0 {
		slices.Sort(conflicts)
		return result, &ConflictError{Paths: conflicts}
	}

	slices.Sort(writes)
	slices.Sort(deletes)

	for _, filePath := range writes {
		if err := s.writeFile(filePath, wanted[filePath].treeEntry); err != nil {
			return result, err
		}

		result.Written = append(result.Written, filePath)
	}

	for _, filePath := range deletes {
		err := os.Remove(s.absolutePath(filePath))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, fmt.Errorf("remove %q: %w", filePath, err)
		}

		result.Removed = append(result.Removed, filePath)
	}

	idx, err := s.loadIndex()
	if err != nil {
		return result, err
	}

	for name := range current {
		if _, kept := target[name]; !kept {
			idx.Remove(name) // not in the index if the user untracked it
		}
	}

	for name, entry := range target {
		setEntry(idx, name, entry.hash, entry.mode, 0, time.Time{})
	}

	return result, s.saveIndex(idx)
}

// traversesSymlink reports whether a parent directory of filePath is itself
// a symlink among files.
func (s *Store) traversesSymlink(filePath string, files map[string]shownEntry) bool {
	for parent := path.Dir(filePath); parent != "." && parent != "/"; parent = path.Dir(parent) {
		if files[parent].mode == filemode.Symlink {
			return true
		}
	}

	return false
}

func (s *Store) writeFile(name string, entry treeEntry) error {
	data, err := s.blobContents(entry.hash)
	if err != nil {
		return err
	}

	filename := s.absolutePath(name)

	if entry.mode == filemode.Symlink {
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			return fmt.Errorf("create directory for %q: %w", name, err)
		}

		if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("replace %q: %w", name, err)
		}

		if err := os.Symlink(string(data), filename); err != nil {
			return fmt.Errorf("create symlink %q: %w", name, err)
		}

		return nil
	}

	permissions := os.FileMode(0o644)
	if entry.mode == filemode.Executable {
		permissions = 0o755
	}

	// Keep permissions the user chose, e.g. 0600 for private files.
	if info, err := os.Lstat(filename); err == nil && info.Mode().IsRegular() {
		permissions = info.Mode().Perm()

		if entry.mode == filemode.Executable {
			// Make the file executable for everyone who can read it.
			permissions |= (permissions & 0o444) >> 2
		}
	}

	if err := atomicfile.WriteFile(filename, data, permissions); err != nil {
		return fmt.Errorf("write %q: %w", name, err)
	}

	return nil
}
