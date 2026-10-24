package store

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// legacyStateFile recorded the links that dotmgr 0.3 created for variants.
const legacyStateFile = "dotmgr-alt.json"

// UpgradeLinkedVariants converts a home directory set up by dotmgr 0.3, in
// which every variant was a file named <path>##<conditions> and <path> was a
// symbolic link to (or a copy of) the one in use, to the current layout, in
// which only <path> exists. It returns what it did, for the user.
//
// A variant file that is still linked is moved to <path>, keeping any local
// changes. Other variant files are deleted when their content is in the
// repository, and kept otherwise.
func (s *Store) UpgradeLinkedVariants() ([]string, error) {
	stateFile := filepath.Join(s.gitDir, legacyStateFile)

	if _, err := os.Stat(stateFile); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	files := indexFiles(idx)
	var notes []string

	for _, name := range slices.Sorted(maps.Keys(files)) {
		filePath, conditions := SplitName(name)
		if conditions == nil {
			continue
		}

		variantFile := s.absolutePath(name)
		if _, err := os.Lstat(variantFile); err != nil {
			continue
		}

		link := s.absolutePath(filePath)

		if target, err := os.Readlink(link); err == nil && target == filepath.Base(variantFile) {
			if err := os.Remove(link); err != nil {
				return notes, fmt.Errorf("remove link %q: %w", filePath, err)
			}

			if err := os.Rename(variantFile, link); err != nil {
				return notes, fmt.Errorf("move %q to %q: %w", name, filePath, err)
			}

			notes = append(notes, fmt.Sprintf("moved %s to %s", name, filePath))

			continue
		}

		file, err := s.readWorktree(name, files[name].mode)
		if err != nil || file.hash() != files[name].hash {
			notes = append(notes, fmt.Sprintf(
				"kept %s: it has changes that are not in the repository; "+
					"move them into %s and delete it",
				name, filePath,
			))

			continue
		}

		if err := os.Remove(variantFile); err != nil {
			return notes, fmt.Errorf("remove %q: %w", name, err)
		}

		notes = append(notes, fmt.Sprintf("removed %s, which is kept in the repository", name))
	}

	if err := os.Remove(stateFile); err != nil {
		return notes, fmt.Errorf("remove %q: %w", stateFile, err)
	}

	return notes, nil
}
