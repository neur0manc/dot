package tracking

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git.sr.ht/~stephan/dotmgr/internal/manifest"
)

// LocalState describes the relationship between a repository source and its
// deployed target on the host.
type LocalState string

const (
	// LocalClean means repository source and deployed target are identical.
	LocalClean LocalState = "clean"

	// LocalModified means both files exist, but their contents differ.
	// It deliberately does not claim which side changed.
	LocalModified LocalState = "modified"

	// LocalNotApplied means the repository source exists, but the deployed
	// target does not.
	LocalNotApplied LocalState = "not-applied"

	// LocalSourceMissing means the manifest references a repository source
	// that does not exist.
	LocalSourceMissing LocalState = "source-missing"

	// LocalUnsupported means either side is not a regular file.
	LocalUnsupported LocalState = "unsupported"
)

type LocalFileStatus struct {
	Entry manifest.File
	State LocalState
}

// LocalStatuses compares every manifest entry with its deployed host file.
//
// The repository worktree is treated as dotmgr-managed internal state.
// The deployed targets are the user-facing configuration files.
func LocalStatuses(
	repositoryPath string,
	homeDirectory string,
) ([]LocalFileStatus, error) {
	currentManifest, err := manifest.Load(manifest.Path(repositoryPath))
	if err != nil {
		return nil, err
	}

	resolvedRepository, err := canonicalDirectory(repositoryPath)
	if err != nil {
		return nil, fmt.Errorf("resolve repository directory: %w", err)
	}

	resolvedHome, err := canonicalDirectory(homeDirectory)
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}

	statuses := make([]LocalFileStatus, 0, len(currentManifest.Files))

	for _, entry := range currentManifest.Files {
		sourcePath, err := resolveRepositorySource(
			resolvedRepository,
			entry.Source,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"resolve manifest source %q: %w",
				entry.Source,
				err,
			)
		}

		targetPath, err := resolveHomeTarget(
			resolvedHome,
			entry.Target,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"resolve manifest target %q: %w",
				entry.Target,
				err,
			)
		}

		state, err := compareManagedFiles(sourcePath, targetPath)
		if err != nil {
			return nil, fmt.Errorf(
				"compare %q with %q: %w",
				entry.Source,
				entry.Target,
				err,
			)
		}

		statuses = append(statuses, LocalFileStatus{
			Entry: entry,
			State: state,
		})
	}

	return statuses, nil
}

// resolveRepositorySource resolves a slash-separated repository path while
// preventing manifest entries from escaping the repository.
func resolveRepositorySource(
	repositoryPath string,
	source string,
) (string, error) {
	if source == "" {
		return "", fmt.Errorf("source must not be empty")
	}

	nativeSource := filepath.FromSlash(source)

	if filepath.IsAbs(nativeSource) {
		return "", fmt.Errorf("source must be repository-relative")
	}

	cleaned := filepath.Clean(nativeSource)

	if cleaned == "." ||
		cleaned == ".." ||
		strings.HasPrefix(
			cleaned,
			".."+string(filepath.Separator),
		) {
		return "", fmt.Errorf("source escapes the repository")
	}

	resolved := filepath.Join(repositoryPath, cleaned)

	if _, err := relativeContainedPath(repositoryPath, resolved); err != nil {
		return "", fmt.Errorf("source escapes the repository: %w", err)
	}

	return resolved, nil
}

// resolveHomeTarget currently supports portable ${HOME}/... targets.
//
// This is intentionally kept separate from repository source resolution.
// Later, additional roots such as ${CONFIG} and ${APPDATA} can be added here.
func resolveHomeTarget(
	homeDirectory string,
	target string,
) (string, error) {
	const homePrefix = "${HOME}/"

	if !strings.HasPrefix(target, homePrefix) {
		return "", fmt.Errorf(
			"unsupported target %q; expected a ${HOME}/ target",
			target,
		)
	}

	relativeTarget := strings.TrimPrefix(target, homePrefix)
	nativeTarget := filepath.FromSlash(relativeTarget)

	if nativeTarget == "" {
		return "", fmt.Errorf("target path must not be empty")
	}

	if filepath.IsAbs(nativeTarget) {
		return "", fmt.Errorf("target must be relative to ${HOME}")
	}

	cleaned := filepath.Clean(nativeTarget)

	if cleaned == "." ||
		cleaned == ".." ||
		strings.HasPrefix(
			cleaned,
			".."+string(filepath.Separator),
		) {
		return "", fmt.Errorf("target escapes the home directory")
	}

	resolved := filepath.Join(homeDirectory, cleaned)

	if _, err := relativeContainedPath(homeDirectory, resolved); err != nil {
		return "", fmt.Errorf("target escapes the home directory: %w", err)
	}

	return resolved, nil
}

func compareManagedFiles(
	sourcePath string,
	targetPath string,
) (LocalState, error) {
	sourceInfo, err := os.Lstat(sourcePath)
	if errors.Is(err, os.ErrNotExist) {
		return LocalSourceMissing, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect repository source: %w", err)
	}

	if !sourceInfo.Mode().IsRegular() {
		return LocalUnsupported, nil
	}

	targetInfo, err := os.Lstat(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		return LocalNotApplied, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect deployed target: %w", err)
	}

	if !targetInfo.Mode().IsRegular() {
		return LocalUnsupported, nil
	}

	sourceData, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("read repository source: %w", err)
	}

	targetData, err := os.ReadFile(targetPath)
	if err != nil {
		return "", fmt.Errorf("read deployed target: %w", err)
	}

	if bytes.Equal(sourceData, targetData) {
		return LocalClean, nil
	}

	return LocalModified, nil
}
