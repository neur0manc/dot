package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

const repositoryEnvironmentVariable = "DOTMGR_REPO"

// Repository returns the directory containing dotmgr's local Git repository.
//
// The DOTMGR_REPO environment variable can override the default location.
// This is useful for testing and for users who want to place the repository
// somewhere else.
func Repository() (string, error) {
	if configured := os.Getenv(repositoryEnvironmentVariable); configured != "" {
		absolute, err := filepath.Abs(configured)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w",
				repositoryEnvironmentVariable, err)
		}

		return filepath.Clean(absolute), nil
	}

	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("determine user config directory: %w", err)
	}

	return filepath.Join(configDirectory, "dotmgr", "repository"), nil
}
