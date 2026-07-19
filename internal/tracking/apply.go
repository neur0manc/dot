package tracking

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"git.sr.ht/~stephan/dotmgr/internal/atomicfile"
	"git.sr.ht/~stephan/dotmgr/internal/gitstore"
	"git.sr.ht/~stephan/dotmgr/internal/manifest"
)

var ErrApplyConflicts = errors.New(
	"managed host files differ from repository sources",
)

// ApplyConflictError reports files that apply refused to overwrite.
type ApplyConflictError struct {
	Targets []string
}

func (e *ApplyConflictError) Error() string {
	return fmt.Sprintf(
		"%v: %s; rerun with --force to overwrite them",
		ErrApplyConflicts,
		strings.Join(e.Targets, ", "),
	)
}

func (e *ApplyConflictError) Unwrap() error {
	return ErrApplyConflicts
}

// ApplyResult summarizes a completed apply operation.
type ApplyResult struct {
	Applied     []manifest.File
	Overwritten []manifest.File
	Skipped     []manifest.File
}

type pendingApply struct {
	entry       manifest.File
	targetPath  string
	data        []byte
	mode        os.FileMode
	overwriting bool
}

// Apply copies managed repository files to their host targets.
//
// Missing targets are created. Identical targets are skipped. Differing
// targets are rejected unless force is true.
//
// All entries are checked before any target is written. Each individual file
// is replaced atomically, but the complete multi-file operation is not a
// filesystem transaction.
func Apply(
	repositoryPath string,
	homeDirectory string,
	force bool,
) (ApplyResult, error) {
	var result ApplyResult

	if err := gitstore.Validate(repositoryPath); err != nil {
		return result, err
	}

	currentManifest, err := manifest.Load(
		manifest.Path(repositoryPath),
	)
	if err != nil {
		return result, err
	}

	resolvedRepository, err := canonicalDirectory(repositoryPath)
	if err != nil {
		return result, fmt.Errorf(
			"resolve repository directory: %w",
			err,
		)
	}

	resolvedHome, err := canonicalDirectory(homeDirectory)
	if err != nil {
		return result, fmt.Errorf(
			"resolve home directory: %w",
			err,
		)
	}

	pending := make([]pendingApply, 0, len(currentManifest.Files))
	conflicts := make([]string, 0)
	targets := make(map[string]string)

	for _, entry := range currentManifest.Files {
		sourcePath, err := resolveRepositorySource(
			resolvedRepository,
			entry.Source,
		)
		if err != nil {
			return result, fmt.Errorf(
				"resolve manifest source %q: %w",
				entry.Source,
				err,
			)
		}

		if err := rejectSymlinkComponents(
			resolvedRepository,
			sourcePath,
		); err != nil {
			return result, fmt.Errorf(
				"validate repository source %q: %w",
				entry.Source,
				err,
			)
		}

		sourceInfo, err := os.Lstat(sourcePath)
		if errors.Is(err, os.ErrNotExist) {
			return result, fmt.Errorf(
				"repository source %q does not exist",
				entry.Source,
			)
		}
		if err != nil {
			return result, fmt.Errorf(
				"inspect repository source %q: %w",
				entry.Source,
				err,
			)
		}

		if !sourceInfo.Mode().IsRegular() {
			return result, fmt.Errorf(
				"repository source %q is not a regular file",
				entry.Source,
			)
		}

		sourceData, err := os.ReadFile(sourcePath)
		if err != nil {
			return result, fmt.Errorf(
				"read repository source %q: %w",
				entry.Source,
				err,
			)
		}

		targetPath, err := resolveHomeTarget(
			resolvedHome,
			entry.Target,
		)
		if err != nil {
			return result, fmt.Errorf(
				"resolve manifest target %q: %w",
				entry.Target,
				err,
			)
		}

		targetKey := applyTargetKey(targetPath)
		if previous, exists := targets[targetKey]; exists {
			return result, fmt.Errorf(
				"manifest targets %q and %q resolve to the same path",
				previous,
				entry.Target,
			)
		}
		targets[targetKey] = entry.Target

		if err := rejectSymlinkComponents(
			resolvedHome,
			targetPath,
		); err != nil {
			return result, fmt.Errorf(
				"validate deployed target %q: %w",
				entry.Target,
				err,
			)
		}

		targetInfo, err := os.Lstat(targetPath)
		if errors.Is(err, os.ErrNotExist) {
			pending = append(pending, pendingApply{
				entry:      entry,
				targetPath: targetPath,
				data:       sourceData,
				mode:       sourceInfo.Mode().Perm(),
			})
			continue
		}
		if err != nil {
			return result, fmt.Errorf(
				"inspect deployed target %q: %w",
				entry.Target,
				err,
			)
		}

		if !targetInfo.Mode().IsRegular() {
			return result, fmt.Errorf(
				"deployed target %q is not a regular file",
				entry.Target,
			)
		}

		targetData, err := os.ReadFile(targetPath)
		if err != nil {
			return result, fmt.Errorf(
				"read deployed target %q: %w",
				entry.Target,
				err,
			)
		}

		if bytes.Equal(sourceData, targetData) {
			result.Skipped = append(result.Skipped, entry)
			continue
		}

		if !force {
			conflicts = append(conflicts, entry.Target)
			continue
		}

		// Preserve the existing target's permissions when overwriting it.
		pending = append(pending, pendingApply{
			entry:       entry,
			targetPath:  targetPath,
			data:        sourceData,
			mode:        targetInfo.Mode().Perm(),
			overwriting: true,
		})
	}

	if len(conflicts) != 0 {
		return result, &ApplyConflictError{
			Targets: conflicts,
		}
	}

	for _, item := range pending {
		if err := atomicfile.WriteFile(
			item.targetPath,
			item.data,
			item.mode,
		); err != nil {
			successfulWrites := len(result.Applied) +
				len(result.Overwritten)

			return result, fmt.Errorf(
				"apply %q after %d successful writes: %w",
				item.entry.Target,
				successfulWrites,
				err,
			)
		}

		if item.overwriting {
			result.Overwritten = append(
				result.Overwritten,
				item.entry,
			)
		} else {
			result.Applied = append(
				result.Applied,
				item.entry,
			)
		}
	}

	return result, nil
}

// rejectSymlinkComponents rejects symbolic links anywhere between base and
// candidate, including candidate itself when it already exists.
//
// Missing components are accepted because apply may create the required
// directories and target file.
func rejectSymlinkComponents(
	base string,
	candidate string,
) error {
	relative, err := relativeContainedPath(base, candidate)
	if err != nil {
		return err
	}

	current := base

	for _, component := range splitNativePath(relative) {
		current = filepath.Join(current, component)

		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf(
				"inspect path component %q: %w",
				current,
				err,
			)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf(
				"path component %q is a symbolic link",
				current,
			)
		}
	}

	return nil
}

func splitNativePath(path string) []string {
	return strings.FieldsFunc(
		path,
		func(character rune) bool {
			return character == '/' || character == '\\'
		},
	)
}

func applyTargetKey(path string) string {
	path = filepath.Clean(path)

	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}

	return path
}
