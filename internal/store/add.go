package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// resolve turns a user-supplied path into an absolute path and the
// slash-separated name relative to the working tree.
//
// Symbolic links in the parent directories are resolved, but a final
// symlink is kept, because symlinks are tracked as links.
func (s *Store) resolve(argument string) (string, string, error) {
	if argument == "" {
		return "", "", errors.New("path must not be empty")
	}

	expanded := argument

	if argument == "~" {
		expanded = s.home
	} else if strings.HasPrefix(argument, "~/") ||
		strings.HasPrefix(argument, `~\`) {
		expanded = filepath.Join(s.home, argument[2:])
	}

	absolute, err := filepath.Abs(expanded)
	if err != nil {
		return "", "", fmt.Errorf("resolve %q: %w", argument, err)
	}

	canonicalHome, err := filepath.EvalSymlinks(s.home)
	if err != nil {
		return "", "", fmt.Errorf("resolve home directory: %w", err)
	}

	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", "", fmt.Errorf("resolve %q: %w", argument, err)
	}

	full := filepath.Join(parent, filepath.Base(absolute))

	name, ok := relativeTo(canonicalHome, full)
	if !ok {
		return "", "", fmt.Errorf(
			"%q is not beneath the home directory %q",
			argument,
			canonicalHome,
		)
	}

	return full, name, nil
}

// relativeTo returns candidate relative to base, as a slash-separated name.
// It reports false when candidate is base itself or lies outside of it.
func relativeTo(base string, candidate string) (string, bool) {
	relative, err := filepath.Rel(base, candidate)
	if err != nil ||
		relative == "." ||
		relative == ".." ||
		filepath.IsAbs(relative) ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}

	return filepath.ToSlash(relative), true
}

// Add stages the given files, or all files beneath the given directories.
// It returns the names that were staged.
func (s *Store) Add(arguments ...string) ([]string, error) {
	if len(arguments) == 0 {
		return nil, errors.New("no paths given")
	}

	canonicalGitDir, err := filepath.EvalSymlinks(s.gitDir)
	if err != nil {
		return nil, fmt.Errorf("resolve repository directory: %w", err)
	}

	var names []string

	for _, argument := range arguments {
		full, name, err := s.resolve(argument)
		if err != nil {
			return nil, err
		}

		if _, inside := relativeTo(canonicalGitDir, full); inside ||
			full == canonicalGitDir {
			return nil, fmt.Errorf(
				"cannot track %q because it is inside the dotmgr repository",
				argument,
			)
		}

		collected, err := collectFiles(full, name, canonicalGitDir)
		if err != nil {
			return nil, err
		}

		names = append(names, collected...)
	}

	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	names = unique(names)

	for _, name := range names {
		if err := validateName(name); err != nil {
			return nil, err
		}

		file, err := s.readWorktree(name, 0)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", name, err)
		}

		hash, err := s.writeBlob(file.data)
		if err != nil {
			return nil, err
		}

		setEntry(idx, name, hash, file.mode, file.size, file.modTime)
	}

	if err := s.saveIndex(idx); err != nil {
		return nil, err
	}

	return names, nil
}

// collectFiles lists the trackable files at full. A directory is walked
// recursively; the repository directory and ".git" directories are skipped.
func collectFiles(
	full string,
	name string,
	gitDir string,
) ([]string, error) {
	info, err := os.Lstat(full)
	if err != nil {
		return nil, fmt.Errorf("inspect %q: %w", full, err)
	}

	if !info.IsDir() {
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return nil, fmt.Errorf(
				"%q is not a regular file, symlink or directory",
				full,
			)
		}

		return []string{name}, nil
	}

	var names []string

	err = filepath.WalkDir(full, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if entry.Name() == ".git" || path == gitDir {
				return filepath.SkipDir
			}

			return nil
		}

		if !entry.Type().IsRegular() && entry.Type()&fs.ModeSymlink == 0 {
			return nil
		}

		relative, err := filepath.Rel(full, path)
		if err != nil {
			return err
		}

		names = append(names, name+"/"+filepath.ToSlash(relative))

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %q: %w", full, err)
	}

	return names, nil
}

func unique(names []string) []string {
	sort.Strings(names)

	result := names[:0]

	for index, name := range names {
		if index == 0 || name != names[index-1] {
			result = append(result, name)
		}
	}

	return result
}

// Remove stops tracking the given files, or all tracked files beneath the
// given directories. Files in the working tree are left alone.
func (s *Store) Remove(arguments ...string) ([]string, error) {
	if len(arguments) == 0 {
		return nil, errors.New("no paths given")
	}

	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	var removed []string

	for _, argument := range arguments {
		_, name, err := s.resolve(argument)
		if err != nil {
			return nil, err
		}

		found := false

		for _, entry := range idx.Entries {
			if entry.Name == name || strings.HasPrefix(entry.Name, name+"/") {
				removed = append(removed, entry.Name)
				found = true
			}
		}

		if !found {
			return nil, fmt.Errorf("%q is not tracked", argument)
		}
	}

	removed = unique(removed)

	for _, name := range removed {
		if _, err := idx.Remove(name); err != nil {
			return nil, fmt.Errorf("untrack %q: %w", name, err)
		}
	}

	if err := s.saveIndex(idx); err != nil {
		return nil, err
	}

	return removed, nil
}

// Tracked returns the names of all tracked files, sorted.
func (s *Store) Tracked() ([]string, error) {
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(idx.Entries))

	for _, entry := range idx.Entries {
		names = append(names, entry.Name)
	}

	sort.Strings(names)

	return names, nil
}
