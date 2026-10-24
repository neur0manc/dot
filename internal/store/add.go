package store

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// resolve turns a user-supplied path into an absolute path and the
// slash-separated name relative to the working tree.
//
// Symbolic links in the parent directories are resolved, but a final
// symlink is kept, because symlinks are tracked as links.
func (s *Store) resolve(argument string) (string, string, error) {
	full, name, err := s.locate(argument)
	if err == nil && name == "" {
		return "", "", fmt.Errorf("%q is the home directory itself", argument)
	}

	return full, name, err
}

// locate is resolve, but also accepts the home directory itself, for which
// it returns an empty name.
func (s *Store) locate(argument string) (string, string, error) {
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

	if full == canonicalHome {
		return full, "", nil
	}

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

// AddedFile describes a file that Add staged.
type AddedFile struct {
	Name     string // the tracked name, e.g. ".zshrc##os.darwin"
	Used     bool   // false for a variant that does not fit this machine
	Replaces string // the plain file that became this variant, if any
}

// Add stages the given files, or all files beneath the given directories.
// A file that has variants updates the variant used on this machine; if
// none fits, the file becomes the plain fallback for such machines.
func (s *Store) Add(arguments ...string) ([]AddedFile, error) {
	return s.add(nil, arguments)
}

// AddVariant stages the given files as the variant for conditions, built
// with Facts.Condition.
//
// When this machine fits the conditions and the file so far was tracked
// plainly, without variants, the plain file is replaced: the file is limited
// to such machines. Otherwise the variant is stored next to what is tracked
// already, so a variant for other machines keeps the plain file as the
// fallback.
func (s *Store) AddVariant(conditions []string, arguments ...string) ([]AddedFile, error) {
	if len(conditions) == 0 {
		return nil, errors.New("no conditions given")
	}

	return s.add(conditions, arguments)
}

func (s *Store) add(conditions []string, arguments []string) ([]AddedFile, error) {
	if len(arguments) == 0 {
		return nil, errors.New("no paths given")
	}

	canonicalGitDir, err := filepath.EvalSymlinks(s.gitDir)
	if err != nil {
		return nil, fmt.Errorf("resolve repository directory: %w", err)
	}

	var paths []string

	for _, argument := range arguments {
		full, filePath, err := s.resolve(argument)
		if err != nil {
			return nil, err
		}

		if _, inside := relativeTo(canonicalGitDir, full); inside || full == canonicalGitDir {
			return nil, fmt.Errorf("cannot track %q because it is inside the repository", argument)
		}

		collected, err := collectFiles(full, filePath, canonicalGitDir)
		if err != nil {
			return nil, err
		}

		paths = append(paths, collected...)
	}

	slices.Sort(paths)
	paths = slices.Compact(paths)

	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	files := indexFiles(idx)
	shown := s.facts.show(files)

	var added []AddedFile

	for _, filePath := range paths {
		if err := validateName(filePath); err != nil {
			return nil, err
		}

		file, err := s.readWorktree(filePath, shown[filePath].mode)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", filePath, err)
		}

		hash, err := s.writeBlob(file.data)
		if err != nil {
			return nil, err
		}

		result := AddedFile{Name: VariantName(filePath, nil)}

		switch {
		case conditions != nil:
			result.Name = VariantName(filePath, conditions)

			_, fitsHere := s.facts.score(conditions)
			variants := variantsOf(files, filePath)

			if plain := VariantName(filePath, nil); fitsHere && len(variants) == 1 && variants[0] == plain {
				idx.Remove(plain)
				result.Replaces = plain
			}
		case shown[filePath].name != "":
			result.Name = shown[filePath].name
		}

		setEntry(idx, result.Name, hash, file.mode, file.size, file.modTime)
		added = append(added, result)
	}

	if err := s.saveIndex(idx); err != nil {
		return nil, err
	}

	shown = s.facts.show(indexFiles(idx))
	for index, result := range added {
		filePath, _ := SplitName(result.Name)
		added[index].Used = shown[filePath].name == result.Name
	}

	return added, nil
}

// variantsOf returns the tracked names that are stored for filePath: the
// plain file and its variants.
func variantsOf(files map[string]treeEntry, filePath string) []string {
	var names []string

	for name := range files {
		if p, _ := SplitName(name); p == filePath {
			names = append(names, name)
		}
	}

	slices.Sort(names)

	return names
}

// collectFiles lists the trackable files at full. A directory is walked
// recursively; the repository directory and ".git" directories are skipped.
func collectFiles(full string, name string, gitDir string) ([]string, error) {
	info, err := os.Lstat(full)
	if err != nil {
		return nil, fmt.Errorf("inspect %q: %w", full, err)
	}

	if !info.IsDir() {
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return nil, fmt.Errorf("%q is not a regular file, symlink or directory", full)
		}

		return []string{name}, checkPlainName(name)
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

		name := name + "/" + filepath.ToSlash(relative)
		names = append(names, name)

		return checkPlainName(name)
	})
	if err != nil {
		return nil, fmt.Errorf("walk %q: %w", full, err)
	}

	return names, nil
}

// checkPlainName rejects files whose names look like variants.
func checkPlainName(name string) error {
	if _, conditions := SplitName(name); conditions != nil {
		return fmt.Errorf(
			"%q has %q in its name, which marks variants in the repository; "+
				"rename it and add it with conditions instead",
			name, variantSeparator,
		)
	}

	return nil
}

// Remove stops tracking the given files, or all tracked files and variants
// beneath the given directories. For a file with variants it removes the
// variant for conditions, or the variant used on this machine when
// conditions is empty. Files on disk are left alone.
func (s *Store) Remove(conditions []string, arguments ...string) ([]string, error) {
	if len(arguments) == 0 {
		return nil, errors.New("no paths given")
	}

	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	files := indexFiles(idx)
	shown := s.facts.show(files)

	var removed []string

	for _, argument := range arguments {
		_, filePath, err := s.resolve(argument)
		if err != nil {
			return nil, err
		}

		before := len(removed)

		for name := range files {
			if p, _ := SplitName(name); strings.HasPrefix(p, filePath+"/") {
				removed = append(removed, name)
			}
		}

		variants := variantsOf(files, filePath)

		switch {
		case len(conditions) > 0:
			name := VariantName(filePath, conditions)
			if !slices.Contains(variants, name) {
				return nil, fmt.Errorf("%s is not tracked", Describe(name))
			}

			removed = append(removed, name)
		case shown[filePath].name != "":
			removed = append(removed, shown[filePath].name)
		case len(variants) > 0:
			return nil, fmt.Errorf(
				"no variant of %q is used on this machine; say which one to remove",
				argument,
			)
		}

		if len(removed) == before {
			return nil, fmt.Errorf("%q is not tracked", argument)
		}
	}

	slices.Sort(removed)
	removed = slices.Compact(removed)

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

// TrackedFile is a tracked file or variant.
type TrackedFile struct {
	Name string // the tracked name, e.g. ".zshrc##os.darwin"
	Path string // the path on disk, e.g. ".zshrc"
	Used bool   // whether this machine uses it
}

// Tracked lists the tracked files and variants at or beneath the given
// paths, or all of them without paths, sorted by name.
func (s *Store) Tracked(arguments ...string) ([]TrackedFile, error) {
	var prefixes []string

	for _, argument := range arguments {
		_, name, err := s.locate(argument)
		if err != nil {
			return nil, err
		}

		if name == "" { // the home directory: everything
			prefixes = nil
			break
		}

		prefixes = append(prefixes, name)
	}

	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	files := indexFiles(idx)
	shown := s.facts.show(files)

	tracked := make([]TrackedFile, 0, len(files))

	for _, name := range slices.Sorted(maps.Keys(files)) {
		filePath, _ := SplitName(name)
		if len(prefixes) > 0 && !slices.ContainsFunc(prefixes, func(prefix string) bool {
			return filePath == prefix || strings.HasPrefix(filePath, prefix+"/")
		}) {
			continue
		}

		tracked = append(tracked, TrackedFile{
			Name: name,
			Path: filePath,
			Used: shown[filePath].name == name,
		})
	}

	return tracked, nil
}
