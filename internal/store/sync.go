package store

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"

	"git.sr.ht/~stephan/dotmgr/internal/atomicfile"
)

// ErrConflicts is wrapped by ConflictError.
var ErrConflicts = errors.New(
	"local files differ from the repository",
)

// ConflictError lists files that Sync refused to overwrite or delete.
type ConflictError struct {
	Paths []string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf(
		"%v: %s",
		ErrConflicts,
		strings.Join(e.Paths, ", "),
	)
}

func (e *ConflictError) Unwrap() error { return ErrConflicts }

// SyncResult summarizes what Sync changed in the working tree.
type SyncResult struct {
	Written []string
	Removed []string
}

type syncAction int

const (
	actionWrite  syncAction = iota
	actionAdopt             // working tree already matches; only update index
	actionDrop              // remove from the index only
	actionDelete            // delete the file and remove it from the index
)

type plannedChange struct {
	name   string
	action syncAction
	target treeEntry
}

// Sync brings the working tree and the index in line with commit target.
//
// With restore, files that already match the index are checked too, so
// deleted or modified files are put back; without it only files that differ
// between the index and target are touched, as after a fast-forward.
//
// A local file is overwritten or deleted only when it is unmodified, that is,
// identical to its index entry or already identical to the target. Anything
// else is a conflict. Conflicts abort the whole operation before any file is
// written, unless force is set.
func (s *Store) Sync(
	target plumbing.Hash,
	restore bool,
	force bool,
) (SyncResult, error) {
	var result SyncResult

	targets, err := s.commitFiles(target)
	if err != nil {
		return result, err
	}

	idx, err := s.loadIndex()
	if err != nil {
		return result, err
	}

	symlinks := map[string]bool{}

	for name, entry := range targets {
		if err := validateName(name); err != nil {
			return result, err
		}

		if entry.mode == filemode.Symlink {
			symlinks[name] = true
		}
	}

	var changes []plannedChange
	var conflicts []string

	for name, wanted := range targets {
		existing := indexEntry(idx, name)

		if existing != nil &&
			existing.Hash == wanted.hash &&
			existing.Mode == wanted.mode &&
			!restore {
			continue
		}

		if traversesSymlink(name, symlinks) {
			return result, fmt.Errorf(
				"refusing to write %q through a symbolic link in the repository",
				name,
			)
		}

		file, err := s.readWorktree(name, wanted.mode)

		switch {
		case errors.Is(err, os.ErrNotExist):
			changes = append(changes, plannedChange{name, actionWrite, wanted})
		case err != nil:
			conflicts = append(conflicts, name)
		case file.hash() == wanted.hash && file.mode == wanted.mode:
			changes = append(changes, plannedChange{name, actionAdopt, wanted})
		case force || (existing != nil && file.hash() == existing.Hash):
			changes = append(changes, plannedChange{name, actionWrite, wanted})
		default:
			conflicts = append(conflicts, name)
		}
	}

	for _, existing := range idx.Entries {
		if _, kept := targets[existing.Name]; kept {
			continue
		}

		file, err := s.readWorktree(existing.Name, existing.Mode)

		switch {
		case errors.Is(err, os.ErrNotExist):
			changes = append(changes, plannedChange{name: existing.Name, action: actionDrop})
		case err != nil:
			conflicts = append(conflicts, existing.Name)
		case force || file.hash() == existing.Hash:
			changes = append(changes, plannedChange{name: existing.Name, action: actionDelete})
		default:
			conflicts = append(conflicts, existing.Name)
		}
	}

	if len(conflicts) > 0 && !force {
		sort.Strings(conflicts)
		return result, &ConflictError{Paths: conflicts}
	}

	sort.Slice(changes, func(left int, right int) bool {
		return changes[left].name < changes[right].name
	})

	applyErr := s.applyChanges(idx, changes, &result)

	if err := s.saveIndex(idx); err != nil {
		return result, errors.Join(applyErr, err)
	}

	return result, applyErr
}

// applyChanges performs the planned changes and records them in idx. It
// stops at the first error; the changes made until then stay in idx.
func (s *Store) applyChanges(
	idx *index.Index,
	changes []plannedChange,
	result *SyncResult,
) error {
	for _, change := range changes {
		switch change.action {
		case actionDrop:
			if _, err := idx.Remove(change.name); err != nil {
				return fmt.Errorf("untrack %q: %w", change.name, err)
			}

		case actionDelete:
			err := os.Remove(s.absolutePath(change.name))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove %q: %w", change.name, err)
			}

			if _, err := idx.Remove(change.name); err != nil {
				return fmt.Errorf("untrack %q: %w", change.name, err)
			}

			result.Removed = append(result.Removed, change.name)

		case actionWrite, actionAdopt:
			if change.action == actionWrite {
				if err := s.writeFile(change.name, change.target); err != nil {
					return err
				}

				result.Written = append(result.Written, change.name)
			}

			file, err := s.readWorktree(change.name, change.target.mode)
			if err != nil {
				return fmt.Errorf("read %q: %w", change.name, err)
			}

			setEntry(
				idx,
				change.name,
				change.target.hash,
				change.target.mode,
				file.size,
				file.modTime,
			)
		}
	}

	return nil
}

// traversesSymlink reports whether a parent directory of name is itself a
// symlink tracked in the repository.
func traversesSymlink(name string, symlinks map[string]bool) bool {
	for parent := path.Dir(name); parent != "." && parent != "/"; parent = path.Dir(parent) {
		if symlinks[parent] {
			return true
		}
	}

	return false
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
