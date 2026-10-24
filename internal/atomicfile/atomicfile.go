// Package atomicfile replaces files so that readers see either the old or the
// new content, never a partial write.
package atomicfile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteFile replaces filename with data, creating missing parent
// directories.
//
// The data is written to a temporary file in the same directory, which is
// then renamed over filename. os.Rename replaces existing files on Windows
// too.
func WriteFile(filename string, data []byte, mode fs.FileMode) error {
	directory := filepath.Dir(filename)

	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create parent directory %q: %w", directory, err)
	}

	tempFile, err := os.CreateTemp(directory, "."+filepath.Base(filename)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}

	tempName := tempFile.Name()
	defer os.Remove(tempName) // fails harmlessly after the rename

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}

	if err := tempFile.Chmod(mode); err != nil {
		tempFile.Close()
		return fmt.Errorf("set temporary file permissions: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return fmt.Errorf("flush temporary file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}

	if err := os.Rename(tempName, filename); err != nil {
		return fmt.Errorf("replace %q: %w", filename, err)
	}

	return nil
}
