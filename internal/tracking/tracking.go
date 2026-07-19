package tracking

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"git.sr.ht/~stephan/dotmgr/internal/atomicfile"
	"git.sr.ht/~stephan/dotmgr/internal/gitstore"
	"git.sr.ht/~stephan/dotmgr/internal/manifest"
)

// Add copies filename into the managed repository, updates the manifest,
// and stages both files in the embedded Git repository.
func Add(
	repositoryPath string,
	homeDirectory string,
	filename string,
) (manifest.File, error) {
	if err := gitstore.Validate(repositoryPath); err != nil {
		return manifest.File{}, err
	}

	resolvedHome, err := canonicalDirectory(homeDirectory)
	if err != nil {
		return manifest.File{}, fmt.Errorf("resolve home directory: %w", err)
	}

	resolvedRepository, err := canonicalDirectory(repositoryPath)
	if err != nil {
		return manifest.File{}, fmt.Errorf("resolve repository directory: %w", err)
	}

	resolvedFile, fileInfo, err := resolveInputFile(filename, resolvedHome)
	if err != nil {
		return manifest.File{}, err
	}

	relativeToHome, err := relativeContainedPath(resolvedHome, resolvedFile)
	if err != nil {
		return manifest.File{}, fmt.Errorf(
			"file %q is not beneath home directory %q: %w",
			resolvedFile,
			resolvedHome,
			err,
		)
	}

	if _, err := relativeContainedPath(resolvedRepository, resolvedFile); err == nil {
		return manifest.File{}, fmt.Errorf(
			"cannot track %q because it is inside the dotmgr repository",
			resolvedFile,
		)
	}

	repositoryRelativeSource := path.Join(
		"files",
		"home",
		filepath.ToSlash(relativeToHome),
	)

	portableTarget := path.Join(
		"${HOME}",
		filepath.ToSlash(relativeToHome),
	)

	entry := manifest.File{
		Source: repositoryRelativeSource,
		Target: portableTarget,
	}

	data, err := os.ReadFile(resolvedFile)
	if err != nil {
		return manifest.File{}, fmt.Errorf("read %q: %w", resolvedFile, err)
	}

	repositoryFile := filepath.Join(
		repositoryPath,
		filepath.FromSlash(repositoryRelativeSource),
	)

	if err := atomicfile.WriteFile(
		repositoryFile,
		data,
		fileInfo.Mode().Perm(),
	); err != nil {
		return manifest.File{}, fmt.Errorf(
			"copy %q into repository: %w",
			resolvedFile,
			err,
		)
	}

	manifestPath := manifest.Path(repositoryPath)

	currentManifest, err := manifest.Load(manifestPath)
	if err != nil {
		return manifest.File{}, err
	}

	currentManifest.Upsert(entry)

	if err := currentManifest.Save(manifestPath); err != nil {
		return manifest.File{}, err
	}

	if err := gitstore.Stage(
		repositoryPath,
		entry.Source,
		manifest.FileName,
	); err != nil {
		return manifest.File{}, err
	}

	return entry, nil
}

func resolveInputFile(
	filename string,
	homeDirectory string,
) (string, os.FileInfo, error) {
	if filename == "" {
		return "", nil, fmt.Errorf("file path must not be empty")
	}

	expanded := filename

	if filename == "~" {
		expanded = homeDirectory
	} else if strings.HasPrefix(filename, "~/") ||
		strings.HasPrefix(filename, `~\`) {
		expanded = filepath.Join(homeDirectory, filename[2:])
	}

	absolute, err := filepath.Abs(expanded)
	if err != nil {
		return "", nil, fmt.Errorf("resolve absolute path %q: %w", filename, err)
	}

	absolute = filepath.Clean(absolute)

	linkInfo, err := os.Lstat(absolute)
	if err != nil {
		return "", nil, fmt.Errorf("inspect %q: %w", absolute, err)
	}

	if linkInfo.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf(
			"cannot track symlink %q; symlink support is not implemented",
			absolute,
		)
	}

	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", nil, fmt.Errorf("resolve %q: %w", absolute, err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("inspect resolved file %q: %w", resolved, err)
	}

	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("%q is not a regular file", resolved)
	}

	return filepath.Clean(resolved), info, nil
}

func canonicalDirectory(directory string) (string, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve symbolic links in %q: %w", absolute, err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect %q: %w", resolved, err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", resolved)
	}

	return filepath.Clean(resolved), nil
}

func relativeContainedPath(base string, candidate string) (string, error) {
	relative, err := filepath.Rel(base, candidate)
	if err != nil {
		return "", err
	}

	if relative == "." {
		return "", fmt.Errorf("path refers to the base directory itself")
	}

	if filepath.IsAbs(relative) ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the base directory")
	}

	return relative, nil
}
