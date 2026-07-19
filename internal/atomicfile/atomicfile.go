package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteFile replaces filename with data.
//
// The temporary file is created in the target directory so that the final
// rename remains on the same filesystem. If the destination already exists,
// it is temporarily moved aside so replacement also works on Windows.
func WriteFile(filename string, data []byte, mode fs.FileMode) error {
	directory := filepath.Dir(filename)

	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create parent directory %q: %w", directory, err)
	}

	tempFile, err := os.CreateTemp(
		directory,
		"."+filepath.Base(filename)+".tmp-*",
	)
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}

	tempName := tempFile.Name()
	defer os.Remove(tempName)

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

	_, err = os.Lstat(filename)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(tempName, filename); err != nil {
			return fmt.Errorf("install file %q: %w", filename, err)
		}

		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect destination %q: %w", filename, err)
	}

	backupFile, err := os.CreateTemp(
		directory,
		"."+filepath.Base(filename)+".backup-*",
	)
	if err != nil {
		return fmt.Errorf("reserve backup name: %w", err)
	}

	backupName := backupFile.Name()

	if err := backupFile.Close(); err != nil {
		os.Remove(backupName)
		return fmt.Errorf("close backup placeholder: %w", err)
	}

	if err := os.Remove(backupName); err != nil {
		return fmt.Errorf("remove backup placeholder: %w", err)
	}

	if err := os.Rename(filename, backupName); err != nil {
		return fmt.Errorf("create backup of %q: %w", filename, err)
	}

	if err := os.Rename(tempName, filename); err != nil {
		rollbackErr := os.Rename(backupName, filename)
		if rollbackErr != nil {
			return fmt.Errorf(
				"install file %q: %w; restoring backup also failed: %v",
				filename,
				err,
				rollbackErr,
			)
		}

		return fmt.Errorf("install file %q: %w", filename, err)
	}

	if err := os.Remove(backupName); err != nil {
		return fmt.Errorf(
			"file was written, but backup %q could not be removed: %w",
			backupName,
			err,
		)
	}

	return nil
}
