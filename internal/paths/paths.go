// Package paths finds the directories dotmgr works with.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const (
	homeEnvironmentVariable       = "DOTMGR_HOME"
	repositoryEnvironmentVariable = "DOTMGR_REPO"
)

// Home returns the directory that acts as dotmgr's working tree.
//
// The DOTMGR_HOME environment variable overrides the user's home directory.
// This is mainly useful for testing.
func Home() (string, error) {
	if configured := os.Getenv(homeEnvironmentVariable); configured != "" {
		return absolute(homeEnvironmentVariable, configured)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determine home directory: %w", err)
	}

	return filepath.Clean(home), nil
}

// Repository returns the directory containing dotmgr's Git directory.
//
// The repository is kept outside of the working tree, so tracked files in the
// home directory never interact with a stray .git directory. The
// DOTMGR_REPO environment variable overrides the default location.
func Repository() (string, error) {
	if configured := os.Getenv(repositoryEnvironmentVariable); configured != "" {
		return absolute(repositoryEnvironmentVariable, configured)
	}

	dataDirectory, err := dataDirectory()
	if err != nil {
		return "", err
	}

	return filepath.Join(dataDirectory, "dotmgr", "repo.git"), nil
}

func dataDirectory() (string, error) {
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Clean(local), nil
		}

		configDirectory, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("determine data directory: %w", err)
		}

		return configDirectory, nil
	}

	if configured := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(configured) {
		return filepath.Clean(configured), nil
	}

	home, err := Home()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".local", "share"), nil
}

func absolute(name string, value string) (string, error) {
	result, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", name, err)
	}

	return filepath.Clean(result), nil
}
