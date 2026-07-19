package tracking

import (
	"bytes"
	"fmt"
	"os"

	"github.com/pmezard/go-difflib/difflib"

	"git.sr.ht/~stephan/dotmgr/internal/manifest"
)

// LocalFileDiff describes a difference between a repository source and its
// deployed target.
type LocalFileDiff struct {
	Entry  manifest.File
	State  LocalState
	Patch  string
	Binary bool
}

// LocalDiffs compares all managed repository sources with their deployed
// targets.
//
// Clean entries are omitted. Repository content is treated as the old side of
// the comparison and deployed host content as the new side.
func LocalDiffs(
	repositoryPath string,
	homeDirectory string,
) ([]LocalFileDiff, error) {
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

	result := make([]LocalFileDiff, 0)

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

		if state == LocalClean {
			continue
		}

		fileDiff := LocalFileDiff{
			Entry: entry,
			State: state,
		}

		switch state {
		case LocalModified:
			sourceData, err := os.ReadFile(sourcePath)
			if err != nil {
				return nil, fmt.Errorf(
					"read repository source %q: %w",
					entry.Source,
					err,
				)
			}

			targetData, err := os.ReadFile(targetPath)
			if err != nil {
				return nil, fmt.Errorf(
					"read deployed target %q: %w",
					entry.Target,
					err,
				)
			}

			fileDiff.Binary = containsNUL(sourceData) ||
				containsNUL(targetData)

			if !fileDiff.Binary {
				fileDiff.Patch, err = unifiedDiff(
					entry,
					sourceData,
					targetData,
				)
				if err != nil {
					return nil, err
				}
			}

		case LocalNotApplied:
			sourceData, err := os.ReadFile(sourcePath)
			if err != nil {
				return nil, fmt.Errorf(
					"read repository source %q: %w",
					entry.Source,
					err,
				)
			}

			fileDiff.Binary = containsNUL(sourceData)

			if !fileDiff.Binary {
				fileDiff.Patch, err = unifiedDiff(
					entry,
					sourceData,
					nil,
				)
				if err != nil {
					return nil, err
				}
			}

		case LocalSourceMissing, LocalUnsupported:
			// These states are reported by the command, but no textual patch
			// can be generated.
		}

		result = append(result, fileDiff)
	}

	return result, nil
}

func unifiedDiff(
	entry manifest.File,
	repositoryData []byte,
	hostData []byte,
) (string, error) {
	patch, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A: difflib.SplitLines(string(repositoryData)),
		B: difflib.SplitLines(string(hostData)),

		FromFile: "repository:" + entry.Source,
		ToFile:   "host:" + entry.Target,

		Context: 3,
	})
	if err != nil {
		return "", fmt.Errorf(
			"create diff for %q: %w",
			entry.Target,
			err,
		)
	}

	return patch, nil
}

func containsNUL(data []byte) bool {
	return bytes.IndexByte(data, 0) >= 0
}
